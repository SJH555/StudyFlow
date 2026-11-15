//go:build (linux && !android) || windows

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/storage"
)

type switchFixture struct {
	db                                       *Database
	old                                      *sql.DB
	root, location, oldPath, target, newPath string
}

func newSwitchFixture(t *testing.T) switchFixture {
	t.Helper()
	root := t.TempDir()
	f := switchFixture{
		root: root, location: filepath.Join(root, "storage-location.txt"),
		oldPath: filepath.Join(root, "old", "study.db"), target: filepath.Join(root, "new"),
		newPath: filepath.Join(root, "new", "study.db"),
	}
	writeDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
	writeDirectoryFixture(t, filepath.Join(filepath.Dir(f.oldPath), "backups", "existing.db"), "old backup")
	var err error
	f.db, err = NewDatabase(f.oldPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	f.old = f.db.db
	if _, err := f.old.Exec(`INSERT INTO settings(key, value) VALUES ('switch-fixture', 'existing data')`); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f switchFixture) run(ops directorySwitchOps) (DirectorySwitchResult, error) {
	return f.db.switchDirectory(context.Background(), f.target, f.location, f.root, ops)
}

func (f switchFixture) pendingPath() string { return f.location + ".pending" }

func (f switchFixture) assertActive(t *testing.T, want string) {
	t.Helper()
	if f.db.Path() != want {
		t.Fatalf("Database.Path() = %q, want %q", f.db.Path(), want)
	}
	if err := f.db.WithRead(func(db *sql.DB, path string) error {
		var seq int
		var name, sqlitePath string
		if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &sqlitePath); err != nil {
			return err
		}
		if path != want || name != "main" || sqlitePath != want {
			return fmt.Errorf("borrowed path %q, SQLite database %q at %q, want %q", path, name, sqlitePath, want)
		}
		var value string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key='switch-fixture'`).Scan(&value); err != nil {
			return err
		}
		if value != "existing data" {
			return fmt.Errorf("existing data = %q", value)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchDirectoryCommitsRealDatabaseFromWAL(t *testing.T) {
	f := newSwitchFixture(t)
	var journal string
	if err := f.old.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal mode = %q, %v", journal, err)
	}
	if _, err := f.old.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.old.Exec(`UPDATE settings SET value='existing data' WHERE key='switch-fixture';
		INSERT INTO settings(key, value) VALUES ('switch-wal-only', 'committed');
		DELETE FROM settings WHERE key='dailyReviewEnabled'`); err != nil {
		t.Fatal(err)
	}
	result, err := f.db.SwitchDirectory(context.Background(), f.target, f.location, f.root)
	if err != nil || !result.Committed || result.Warning != nil || !result.Retired.CleanupAllowed || result.Retired.Path != f.oldPath || result.Retired.Identity == nil {
		t.Fatalf("switch = %+v, %v", result, err)
	}
	f.assertActive(t, f.newPath)
	assertDirectoryFixture(t, f.location, f.target+"\n")
	assertDirectoryMissing(t, f.pendingPath())
	assertDirectoryFixture(t, filepath.Join(f.target, "backups", "existing.db"), "old backup")
	assertDirectoryMissing(t, filepath.Join(filepath.Dir(f.oldPath), "backups", "existing.db"))
	assertDirectoryMissing(t, f.oldPath)
	if err := f.old.Ping(); err == nil {
		t.Fatal("retired connection is still open")
	}
	if err := f.db.WithRead(func(db *sql.DB, _ string) error {
		var value string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key='switch-wal-only'`).Scan(&value); err != nil || value != "committed" {
			return fmt.Errorf("WAL data = %q, %v", value, err)
		}
		if err := db.QueryRow(`SELECT value FROM settings WHERE key='dailyReviewEnabled'`).Scan(&value); !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("candidate gained a startup default: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchDirectoryPrecommitFailuresKeepOldDatabase(t *testing.T) {
	for _, failure := range []string{"prepare", "target sync", "candidate open", "candidate validation", "candidate close during open", "stage without pending", "stage with pending"} {
		t.Run(failure, func(t *testing.T) {
			f := newSwitchFixture(t)
			ops := defaultDirectorySwitchOps()
			injected := errors.New(failure)
			validation := errors.New("candidate validation failed")
			switch failure {
			case "prepare":
				writeDirectoryFixture(t, filepath.Join(f.target, "backups", "existing.db"), "user backup")
			case "target sync":
				ops.prepare = func(ctx context.Context, db *sql.DB, old, target, location string) (*preparedDirectory, error) {
					return prepareDirectoryWithIO(ctx, db, old, target, location, directoryPrepareIO{
						copyBackup: io.Copy, syncFile: (*os.File).Sync, syncDirectory: func(string) error { return injected },
					})
				}
			case "candidate open":
				ops.openCandidate = func(string) (*sql.DB, error) { return nil, injected }
			case "candidate validation":
				if _, err := f.old.Exec(`ALTER TABLE plans RENAME COLUMN note TO missing_note`); err != nil {
					t.Fatal(err)
				}
			case "candidate close during open":
				ops.openCandidate = func(string) (*sql.DB, error) {
					return nil, closeAfterError(validation, func() error { return injected })
				}
			case "stage without pending", "stage with pending":
				ops.stage = func(location *storage.LocationSwitch, old, target, install string) error {
					if failure == "stage with pending" {
						if err := location.Stage(old, target, install); err != nil {
							return err
						}
					}
					return injected
				}
			}
			result, err := f.run(ops)
			if result.Committed || err == nil {
				t.Fatalf("expected failure: %+v, %v", result, err)
			}
			switch failure {
			case "prepare":
				assertDirectoryFixture(t, filepath.Join(f.target, "backups", "existing.db"), "user backup")
			case "candidate validation":
				if !strings.Contains(err.Error(), "t.note") {
					t.Fatalf("expected actual column validation failure, got %v", err)
				}
			default:
				if !errors.Is(err, injected) {
					t.Fatalf("expected injected error, got %v", err)
				}
			}
			if failure == "candidate close during open" || failure == "stage with pending" {
				if _, err := os.Stat(f.newPath); err != nil {
					t.Fatalf("candidate must be retained: %v", err)
				}
			} else {
				assertDirectoryMissing(t, f.newPath)
			}
			if failure == "candidate close during open" && !errors.Is(err, validation) {
				t.Fatalf("validation error lost: %v", err)
			}
			if failure == "stage with pending" {
				if _, err := os.Stat(f.pendingPath()); err != nil {
					t.Fatal(err)
				}
				if _, err := f.run(defaultDirectorySwitchOps()); !errors.Is(err, ErrDirectorySwitchPending) {
					t.Fatalf("subsequent switch = %v", err)
				}
			} else {
				assertDirectoryMissing(t, f.pendingPath())
			}
			f.assertActive(t, f.oldPath)
			assertDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
		})
	}
}

func TestSwitchDirectoryNoOpAndExistingPending(t *testing.T) {
	f := newSwitchFixture(t)
	result, err := f.db.SwitchDirectory(context.Background(), filepath.Dir(f.oldPath), f.location, f.root)
	if err != nil || result.Committed || result.Retired.Path != "" || result.Warning != nil {
		t.Fatalf("same directory switch = %+v, %v", result, err)
	}
	assertDirectoryMissing(t, f.pendingPath())
	f.assertActive(t, f.oldPath)
	prepared, err := prepareDirectory(context.Background(), f.old, f.oldPath, f.target, f.location)
	if err != nil {
		t.Fatal(err)
	}
	location, err := storage.OpenLocationSwitch(f.location)
	if err != nil {
		t.Fatal(err)
	}
	stageErr := location.Stage(f.oldPath, prepared.databasePath, f.root)
	if err := errors.Join(stageErr, location.Close()); err != nil {
		t.Fatal(err)
	}
	ops := defaultDirectorySwitchOps()
	ops.prepare = func(context.Context, *sql.DB, string, string, string) (*preparedDirectory, error) {
		t.Fatal("a pending record must block preparation")
		return nil, nil
	}
	if _, err := f.run(ops); !errors.Is(err, ErrDirectorySwitchPending) {
		t.Fatalf("switch with pending record = %v", err)
	}
	assertDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
	f.assertActive(t, f.oldPath)
}

func TestSwitchDirectoryUnchangedOrdersCandidateCloseBeforeClear(t *testing.T) {
	for _, failure := range []string{"none", "candidate close", "clear"} {
		t.Run(failure, func(t *testing.T) {
			f := newSwitchFixture(t)
			ops := defaultDirectorySwitchOps()
			publishErr := errors.New("publication failed without replacing the location")
			cleanupErr := errors.New(failure)
			var closed, cleared int
			ops.publish = func(location *storage.LocationSwitch) (storage.LocationResolution, error) {
				resolution, err := location.Recover()
				return resolution, errors.Join(publishErr, err)
			}
			ops.closeDB = func(db *sql.DB) error {
				if db == f.old {
					t.Fatal("unchanged publication tried to close the old handle")
				}
				closed++
				err := db.Close()
				if failure == "candidate close" {
					return errors.Join(err, cleanupErr)
				}
				return err
			}
			ops.clear = func(location *storage.LocationSwitch) error {
				cleared++
				if closed != 1 {
					t.Fatal("pending record was cleared before closing the candidate")
				}
				if _, err := os.Stat(f.newPath); err != nil {
					t.Fatalf("candidate files must remain until clear succeeds: %v", err)
				}
				if failure == "clear" {
					return cleanupErr
				}
				return location.Clear()
			}
			result, err := f.run(ops)
			if result.Committed || !errors.Is(err, publishErr) || closed != 1 {
				t.Fatalf("switch = %+v, %v; closed = %d", result, err, closed)
			}
			f.assertActive(t, f.oldPath)
			assertDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
			if failure == "none" {
				if cleared != 1 {
					t.Fatalf("clear called %d times", cleared)
				}
				assertDirectoryMissing(t, f.pendingPath())
				assertDirectoryMissing(t, f.target)
				return
			}
			if !errors.Is(err, cleanupErr) || (failure == "candidate close" && cleared != 0) {
				t.Fatalf("cleanup failure = %v; cleared = %d", err, cleared)
			}
			for _, path := range []string{f.newPath, f.pendingPath()} {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("must retain %s: %v", path, err)
				}
			}
			if _, err := f.run(defaultDirectorySwitchOps()); !errors.Is(err, ErrDirectorySwitchPending) {
				t.Fatalf("subsequent switch = %v", err)
			}
		})
	}
}

func TestSwitchDirectoryCommitKeepsWarningsSeparate(t *testing.T) {
	f := newSwitchFixture(t)
	ops := defaultDirectorySwitchOps()
	publishErr, closeErr := errors.New("publish reported an error"), errors.New("retired close failed")
	var retiredCloses int
	ops.publish = func(location *storage.LocationSwitch) (storage.LocationResolution, error) {
		resolution, err := location.Publish()
		return resolution, errors.Join(err, publishErr)
	}
	ops.closeDB = func(db *sql.DB) error {
		if db != f.old || f.db.db == f.old || f.db.path != f.newPath {
			t.Fatal("old database must close only after assigning the new handle and path")
		}
		retiredCloses++
		return errors.Join(db.Close(), closeErr)
	}
	ops.clear = func(location *storage.LocationSwitch) error {
		t.Fatal("must retain pending record after retired close failure")
		return nil
	}
	result, err := f.run(ops)
	if err != nil || !result.Committed || result.Retired.CleanupAllowed || retiredCloses != 1 {
		t.Fatalf("switch = %+v, %v; retired closes = %d", result, err, retiredCloses)
	}
	if result.Retired.Path != f.oldPath || result.Retired.Identity == nil || !errors.Is(result.Retired.CloseErr, closeErr) {
		t.Fatalf("missing retired database information: %+v", result.Retired)
	}
	for _, warning := range []error{publishErr, closeErr} {
		if !errors.Is(result.Warning, warning) {
			t.Fatalf("missing %v in warnings: %v", warning, result.Warning)
		}
	}
	if _, err := os.Stat(f.pendingPath()); err != nil {
		t.Fatal(err)
	}
	assertDirectoryFixture(t, f.location, f.target+"\n")
	f.assertActive(t, f.newPath)
	if err := f.db.WithWrite(func(db *sql.DB, _ string) error {
		_, err := db.Exec(`INSERT INTO settings(key, value) VALUES ('switch-new-write', 'ok')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(defaultDirectorySwitchOps()); !errors.Is(err, ErrDirectorySwitchPending) {
		t.Fatalf("cleanup uncertainty did not block the next switch: %v", err)
	}
	if err := f.db.Close(); err != nil || retiredCloses != 1 {
		t.Fatalf("close = %v; retired closes = %d", err, retiredCloses)
	}
}

func TestSwitchDirectoryUncertainPausesAndPreservesBothDatabases(t *testing.T) {
	f := newSwitchFixture(t)
	ops := defaultDirectorySwitchOps()
	var candidate *sql.DB
	ops.openCandidate = func(path string) (*sql.DB, error) {
		var err error
		candidate, err = OpenExisting(path)
		return candidate, err
	}
	ops.publish = func(location *storage.LocationSwitch) (storage.LocationResolution, error) {
		writeDirectoryFixture(t, f.location, "external edit\n")
		return location.Publish()
	}
	ops.clear = func(*storage.LocationSwitch) error {
		t.Fatal("uncertain publication tried to clear the pending record")
		return nil
	}
	result, err := f.run(ops)
	if result.Committed || !errors.Is(err, ErrDatabasePaused) || f.db.Path() != f.oldPath {
		t.Fatalf("switch = %+v, %v; path = %q", result, err, f.db.Path())
	}
	if err := f.old.Ping(); err != nil {
		t.Fatalf("old handle should remain open while paused: %v", err)
	}
	if candidate == nil || candidate.Ping() == nil {
		t.Fatal("candidate should be closed while paused")
	}
	unexpected := func(*sql.DB, string) error { t.Fatal("paused database lent a connection"); return nil }
	for _, borrow := range []func(func(*sql.DB, string) error) error{f.db.WithRead, f.db.WithWrite} {
		if err := borrow(unexpected); !errors.Is(err, ErrDatabasePaused) {
			t.Fatalf("borrow while paused = %v", err)
		}
	}
	if _, err := f.run(defaultDirectorySwitchOps()); !errors.Is(err, ErrDatabasePaused) {
		t.Fatalf("switch while paused = %v", err)
	}
	assertDirectoryFixture(t, f.location, "external edit\n")
	for _, path := range []string{f.oldPath, f.newPath, f.pendingPath()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("must retain %s: %v", path, err)
		}
	}
	if err := f.db.Close(); err != nil || f.old.Ping() == nil {
		t.Fatalf("close while paused = %v", err)
	}
}

func TestSwitchDirectoryKeepsCandidateCloseErrorInBlockedState(t *testing.T) {
	for _, stageFails := range []bool{false, true} {
		name := "uncertain publication"
		if stageFails {
			name = "stage with pending"
		}
		t.Run(name, func(t *testing.T) {
			f := newSwitchFixture(t)
			ops := defaultDirectorySwitchOps()
			originalErr := errors.New(name)
			closeErr := errors.New("candidate close failed")
			if stageFails {
				ops.stage = func(location *storage.LocationSwitch, old, target, install string) error {
					if err := location.Stage(old, target, install); err != nil {
						return err
					}
					return originalErr
				}
			} else {
				ops.publish = func(*storage.LocationSwitch) (storage.LocationResolution, error) {
					return storage.LocationResolution{Outcome: storage.LocationUncertain}, originalErr
				}
			}
			ops.closeDB = func(db *sql.DB) error {
				if db == f.old {
					t.Fatal("attempted to close the old database")
				}
				return errors.Join(db.Close(), closeErr)
			}
			result, err := f.run(ops)
			if result.Committed || !errors.Is(err, originalErr) || !errors.Is(err, closeErr) {
				t.Fatalf("initial switch = %+v, %v", result, err)
			}
			var blocked error
			wantState := ErrDatabasePaused
			if stageFails {
				f.assertActive(t, f.oldPath)
				_, blocked = f.run(defaultDirectorySwitchOps())
				wantState = ErrDirectorySwitchPending
			} else {
				blocked = f.db.WithRead(func(*sql.DB, string) error {
					t.Fatal("paused database lent a connection")
					return nil
				})
			}
			if !errors.Is(blocked, wantState) || !errors.Is(blocked, originalErr) || !errors.Is(blocked, closeErr) {
				t.Fatalf("later operation lost the state or its causes: %v", blocked)
			}
		})
	}
}

func TestSwitchDirectoryDoesNotRemoveDataBearingWAL(t *testing.T) {
	f := newSwitchFixture(t)
	ops := defaultDirectorySwitchOps()
	ops.prepare = func(ctx context.Context, db *sql.DB, old, target, location string) (*preparedDirectory, error) {
		prepared, err := prepareDirectory(ctx, db, old, target, location)
		if err == nil {
			writeDirectoryFixture(t, prepared.databasePath+"-wal", "external WAL contents")
		}
		return prepared, err
	}
	ops.openCandidate = func(string) (*sql.DB, error) {
		t.Fatal("a candidate with an unexpected WAL must not be opened")
		return nil, nil
	}
	result, err := f.run(ops)
	if result.Committed || err == nil || !strings.Contains(err.Error(), "-wal") {
		t.Fatalf("switch = %+v, %v", result, err)
	}
	f.assertActive(t, f.oldPath)
	assertDirectoryFixture(t, f.newPath+"-wal", "external WAL contents")
	if _, err := os.Stat(f.newPath); err != nil {
		t.Fatal(err)
	}
	assertDirectoryMissing(t, f.pendingPath())
}

func TestSwitchDirectoryKeepsCandidateDataIfWALGainsFrames(t *testing.T) {
	f := newSwitchFixture(t)
	var mode string
	if err := f.old.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode = %q, %v", mode, err)
	}
	ops := defaultDirectorySwitchOps()
	ops.openCandidate = func(path string) (*sql.DB, error) {
		db, err := OpenExisting(path)
		if err != nil {
			return nil, err
		}
		// 模拟校验通过后出现携带数据的 WAL。
		if _, err := db.Exec(`INSERT INTO settings(key, value) VALUES ('candidate-only', 'retain')`); err != nil {
			return nil, errors.Join(err, db.Close())
		}
		info, err := os.Stat(path + "-wal")
		if err != nil || info.Size() == 0 {
			_ = db.Close()
			t.Fatalf("expected frames in candidate WAL: %v, %v", info, err)
		}
		return db, nil
	}
	result, err := f.run(ops)
	if result.Committed || err == nil || !strings.Contains(err.Error(), "-wal") {
		t.Fatalf("switch = %+v, %v", result, err)
	}
	f.assertActive(t, f.oldPath)
	assertDirectoryMissing(t, f.pendingPath())
	candidate, err := OpenExisting(f.newPath)
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	var value string
	if err := candidate.QueryRow(`SELECT value FROM settings WHERE key='candidate-only'`).Scan(&value); err != nil || value != "retain" {
		t.Fatalf("candidate WAL data was not retained: %q, %v", value, err)
	}
}

func awaitSwitchBarrier(t *testing.T, reached <-chan struct{}) {
	t.Helper()
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for switch barrier")
	}
}

// 仅凭启动信号无法证明借用者已尝试获取锁。需观察特定 goroutine 停在 RWMutex 上，再释放它。
func queueSwitchBorrow(t *testing.T, borrow func(func(*sql.DB, string) error) error, lockMethod string, use func(*sql.DB, string) error) <-chan error {
	t.Helper()
	started := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		var self [128]byte
		n := runtime.Stack(self[:], false)
		id, _, _ := strings.Cut(string(self[:n]), "[")
		started <- id
		done <- borrow(use)
	}()
	var id string
	select {
	case id = <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("borrower did not start")
	}
	stacks := make([]byte, 64*1024)
	waitForDatabaseState(t, func() bool {
		n := runtime.Stack(stacks, true)
		if n == len(stacks) {
			stacks = make([]byte, 2*len(stacks))
			return false
		}
		for _, stack := range strings.Split(string(stacks[:n]), "\n\n") {
			if strings.HasPrefix(stack, id+"[sync.") && strings.Contains(stack, "sync.(*RWMutex)."+lockMethod+"(") {
				return true
			}
		}
		return false
	})
	return done
}

func TestSwitchDirectoryWaitsForActiveBorrowAndLendsOnlyCommittedHandle(t *testing.T) {
	f := newSwitchFixture(t)
	borrowEntered, releaseBorrow := make(chan struct{}), make(chan struct{})
	publishEntered, releasePublish := make(chan struct{}), make(chan struct{})
	unblockBorrow := sync.OnceFunc(func() { close(releaseBorrow) })
	unblockPublish := sync.OnceFunc(func() { close(releasePublish) })
	t.Cleanup(unblockBorrow)
	t.Cleanup(unblockPublish)
	oldRead := make(chan error, 1)
	go func() {
		oldRead <- f.db.WithRead(func(db *sql.DB, path string) error {
			close(borrowEntered)
			<-releaseBorrow
			if db != f.old || path != f.oldPath {
				return fmt.Errorf("active borrow did not retain old handle and path: %q", path)
			}
			return db.Ping()
		})
	}()
	awaitSwitchBarrier(t, borrowEntered)

	ops := defaultDirectorySwitchOps()
	ops.publish = func(location *storage.LocationSwitch) (storage.LocationResolution, error) {
		close(publishEntered)
		<-releasePublish
		return location.Publish()
	}
	switchDone := make(chan error, 1)
	go func() {
		result, err := f.run(ops)
		if err == nil && !result.Committed {
			err = errors.New("switch did not commit")
		}
		switchDone <- errors.Join(err, result.Warning)
	}()
	// 活跃借用持有读锁。TryRLock 失败说明切换写者正在等待它，此时任何准备尚未开始。
	waitForDatabaseState(t, func() bool {
		if f.db.mu.TryRLock() {
			f.db.mu.RUnlock()
			return false
		}
		return true
	})
	assertDirectoryMissing(t, f.target)
	assertDirectoryMissing(t, f.pendingPath())
	if err := f.old.Ping(); err != nil {
		t.Fatalf("old handle closed while borrowed: %v", err)
	}
	unblockBorrow()
	if err := awaitDatabaseResult(t, oldRead); err != nil {
		t.Fatal(err)
	}
	awaitSwitchBarrier(t, publishEntered)

	checkNewHandle := func(db *sql.DB, path string) error {
		if db == f.old || path != f.newPath {
			return fmt.Errorf("borrow received old handle or wrong path: %q", path)
		}
		return nil
	}
	queued := queueSwitchBorrow(t, f.db.WithRead, "RLock", checkNewHandle)
	unblockPublish()
	if err := awaitDatabaseResult(t, switchDone); err != nil {
		t.Fatal(err)
	}
	if err := awaitDatabaseResult(t, queued); err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithRead(checkNewHandle); err != nil {
		t.Fatal(err)
	}
	f.assertActive(t, f.newPath)
	if err := f.old.Ping(); err == nil {
		t.Fatal("retired handle is still open after commit")
	}
}

func TestSwitchDirectoryPauseRejectsQueuedAndNewBorrows(t *testing.T) {
	f := newSwitchFixture(t)
	publishEntered, releasePublish := make(chan struct{}), make(chan struct{})
	unblockPublish := sync.OnceFunc(func() { close(releasePublish) })
	t.Cleanup(unblockPublish)
	publishErr := errors.New("location publication uncertain")
	ops := defaultDirectorySwitchOps()
	ops.publish = func(*storage.LocationSwitch) (storage.LocationResolution, error) {
		close(publishEntered)
		<-releasePublish
		return storage.LocationResolution{Outcome: storage.LocationUncertain}, publishErr
	}
	switchDone := make(chan error, 1)
	go func() {
		_, err := f.run(ops)
		switchDone <- err
	}()
	awaitSwitchBarrier(t, publishEntered)
	unwantedBorrow := func(*sql.DB, string) error { return errors.New("borrow callback ran while paused") }
	queuedReader := queueSwitchBorrow(t, f.db.WithRead, "RLock", unwantedBorrow)
	queuedWriter := queueSwitchBorrow(t, f.db.WithWrite, "Lock", unwantedBorrow)
	unblockPublish()
	for _, result := range []<-chan error{switchDone, queuedReader, queuedWriter} {
		if err := awaitDatabaseResult(t, result); !errors.Is(err, ErrDatabasePaused) || !errors.Is(err, publishErr) {
			t.Fatalf("expected paused state and original publication error, got %v", err)
		}
	}
	for _, borrow := range []func(func(*sql.DB, string) error) error{f.db.WithRead, f.db.WithWrite} {
		if err := borrow(unwantedBorrow); !errors.Is(err, ErrDatabasePaused) || !errors.Is(err, publishErr) {
			t.Fatalf("new borrow while paused = %v", err)
		}
	}
	if f.db.Path() != f.oldPath || f.old.Ping() != nil {
		t.Fatal("pause changed or closed the old handle")
	}
	for _, path := range []string{f.oldPath, f.newPath, f.pendingPath()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("paused switch must retain %s: %v", path, err)
		}
	}
	assertDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
	assertDirectoryFixture(t, filepath.Join(filepath.Dir(f.oldPath), "backups", "existing.db"), "old backup")
	assertDirectoryFixture(t, filepath.Join(f.target, "backups", "existing.db"), "old backup")
}

func TestSwitchDirectoryCloseWhilePausedOnlyClosesCurrentHandle(t *testing.T) {
	f := newSwitchFixture(t)
	ops := defaultDirectorySwitchOps()
	var candidate *sql.DB
	var candidateCloses int
	ops.openCandidate = func(path string) (*sql.DB, error) {
		var err error
		candidate, err = OpenExisting(path)
		return candidate, err
	}
	ops.publish = func(*storage.LocationSwitch) (storage.LocationResolution, error) {
		return storage.LocationResolution{Outcome: storage.LocationUncertain}, errors.New("location publication uncertain")
	}
	ops.closeDB = func(db *sql.DB) error {
		if db != candidate || db == f.old {
			t.Fatal("pause attempted to close a handle other than the candidate")
		}
		candidateCloses++
		return db.Close()
	}
	if _, err := f.run(ops); !errors.Is(err, ErrDatabasePaused) {
		t.Fatalf("switch = %v", err)
	}
	if f.db.db != f.old || f.old.Ping() != nil || candidateCloses != 1 || candidate.Ping() == nil {
		t.Fatal("pause must keep the current handle open and close the uncommitted candidate once")
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	if f.db.db != nil || f.old.Ping() == nil || candidateCloses != 1 {
		t.Fatal("Close did not close only the current handle")
	}
	for _, path := range []string{f.oldPath, f.newPath, f.pendingPath()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Close must retain %s: %v", path, err)
		}
	}
	assertDirectoryFixture(t, filepath.Join(filepath.Dir(f.oldPath), "backups", "existing.db"), "old backup")
	assertDirectoryFixture(t, filepath.Join(f.target, "backups", "existing.db"), "old backup")
}
