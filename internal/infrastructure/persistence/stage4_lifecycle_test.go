//go:build (linux && !android) || windows

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/storage"
)

func TestStage4DirectoryCancellationCommitBoundary(t *testing.T) {
	for _, stage := range []string{"before", "prepared", "staged", "committed"} {
		t.Run(stage, func(t *testing.T) {
			f := newSwitchFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ops := defaultDirectorySwitchOps()
			switch stage {
			case "before":
				cancel()
			case "prepared":
				prepare := ops.prepare
				ops.prepare = func(ctx context.Context, db *sql.DB, a, b, c string) (*preparedDirectory, error) {
					p, err := prepare(ctx, db, a, b, c)
					cancel()
					return p, err
				}
			case "staged":
				stage := ops.stage
				ops.stage = func(l *storage.LocationSwitch, a, b, c string) error { err := stage(l, a, b, c); cancel(); return err }
			case "committed":
				publish := ops.publish
				ops.publish = func(l *storage.LocationSwitch) (storage.LocationResolution, error) {
					r, err := publish(l)
					cancel()
					return r, err
				}
			}
			result, err := f.db.switchDirectory(ctx, f.target, f.location, f.root, ops)
			if stage == "committed" {
				if err != nil || !result.Committed {
					t.Fatalf("committed switch = %+v, %v", result, err)
				}
				f.assertActive(t, f.newPath)
				assertDirectoryFixture(t, f.location, f.target+"\n")
			} else {
				if !errors.Is(err, context.Canceled) || result.Committed {
					t.Fatalf("cancelled switch = %+v, %v", result, err)
				}
				f.assertActive(t, f.oldPath)
				assertDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
				assertDirectoryMissing(t, f.newPath)
			}
			assertDirectoryMissing(t, f.pendingPath())
		})
	}
}

func TestStage4SwitchWaitsForTransactionCleanup(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			f := newSwitchFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			txDone := make(chan error, 1)
			failure := errors.New("business rejected")
			go func() {
				defer func() {
					if p := recover(); p != nil {
						if p != failure {
							t.Errorf("panic=%v", p)
						}
						txDone <- failure
					}
				}()
				txDone <- f.db.WithAccess(context.Background(), func(a *Access) error {
					return a.Queries(context.Background()).WithTx(func(q *Queries) error {
						if _, err := q.Exec(`INSERT INTO settings VALUES('during-tx','saved')`); err != nil {
							return err
						}
						close(entered)
						<-release
						if outcome == "panic" {
							panic(failure)
						}
						if outcome == "rollback" {
							return failure
						}
						return nil
					})
				})
			}()
			<-entered
			switched := make(chan error, 1)
			go func() {
				r, err := f.db.SwitchDirectory(context.Background(), f.target, f.location, f.root)
				if err == nil && !r.Committed {
					err = errors.New("switch not committed")
				}
				switched <- err
			}()
			select {
			case err := <-switched:
				t.Fatalf("switch preceded cleanup: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			if err := <-txDone; (outcome == "commit" && err != nil) || (outcome != "commit" && !errors.Is(err, failure)) {
				t.Fatalf("tx=%v", err)
			}
			if err := <-switched; err != nil {
				t.Fatal(err)
			}
			f.assertActive(t, f.newPath)
			if err := f.db.WithAccess(context.Background(), func(a *Access) error {
				var count int
				if err := a.Queries(context.Background()).QueryRow(`SELECT count(*) FROM settings WHERE key='during-tx'`).Scan(&count); err != nil {
					return err
				}
				want := 0
				if outcome == "commit" {
					want = 1
				}
				if count != want {
					t.Fatalf("snapshot count=%d, want=%d", count, want)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStage4StartupCommitBusyDoesNotLeakMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM settings WHERE key='dailyReviewEnabled'`); err != nil {
		t.Fatal(err)
	}
	reader, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()
	var count int
	if err := reader.QueryRow(`SELECT count(*) FROM settings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenForStartup(path)
	if opened != nil {
		opened.Close()
		t.Fatal("startup exposed uncertain connection")
	}
	if err == nil {
		t.Fatal("expected busy commit")
	}
	requireSQLiteCode(t, err, 5)
	if err := reader.Rollback(); err != nil {
		t.Fatal(err)
	}
	clean, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clean.Close()
	if err := clean.QueryRow(`SELECT count(*) FROM settings WHERE key='dailyReviewEnabled'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("startup partial write=%d, %v", count, err)
	}
}

func TestStage4InitializationPanicRollsBackDDL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	marker := errors.New("initialization panic")
	func() {
		defer func() {
			if p := recover(); p != marker {
				t.Fatalf("panic=%v", p)
			}
		}()
		_ = initializeAtomically(db, func(q *Queries) error {
			if err := initializeSchemaQueries(q); err != nil {
				t.Fatal(err)
			}
			panic(marker)
		})
	}()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("DDL remained=%d, %v", count, err)
	}
	if err := initializeSchema(db); err != nil {
		t.Fatalf("connection unusable after panic cleanup: %v", err)
	}
}

func TestStage4BackupDoesNotOverwriteOrRemoveForeignTemp(t *testing.T) {
	f := newSwitchFixture(t)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	dir := backupDirectory(f.oldPath)
	target := filepath.Join(dir, "study-20261118-120000.db")
	if err := CreateBackup(f.old, f.oldPath, now); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.old.Exec(`INSERT INTO settings VALUES('later','different')`); err != nil {
		t.Fatal(err)
	}
	if err := CreateBackup(f.old, f.oldPath, now); err == nil {
		t.Fatal("overwrote existing backup")
	}
	assertDirectoryFixture(t, target, string(original))
	// Derive exactly the name reserved by CreateBackupContext.
	tmp := filepath.Join(dir, ".study-"+now.Format("20060102-150405")+".db."+fmt.Sprint(now.UnixNano())+".tmp")
	writeDirectoryFixture(t, tmp, "foreign temporary file")
	if err := CreateBackup(f.old, f.oldPath, now); err == nil {
		t.Fatal("expected temp collision")
	}
	assertDirectoryFixture(t, tmp, "foreign temporary file")
}

func TestStage4PausedDatabaseRejectsBackupAndSwitch(t *testing.T) {
	f := newSwitchFixture(t)
	if _, err := f.old.Exec(`CREATE TABLE deferred_probe(subject_id INTEGER REFERENCES subjects(id) DEFERRABLE INITIALLY DEFERRED)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	err := f.db.WithAccess(ctx, func(a *Access) error {
		return a.Queries(ctx).WithTx(func(q *Queries) error { _, err := q.Exec(`INSERT INTO deferred_probe VALUES(999)`); return err })
	})
	if !errors.Is(err, errDatabaseUnavailable) {
		t.Fatalf("commit did not pause database: %v", err)
	}
	ops := defaultDirectorySwitchOps()
	ops.prepare = func(context.Context, *sql.DB, string, string, string) (*preparedDirectory, error) {
		t.Error("paused switch prepared files")
		return nil, errors.New("unexpected prepare")
	}
	if _, err := f.db.switchDirectory(ctx, f.target, f.location, f.root, ops); !errors.Is(err, errDatabaseUnavailable) {
		t.Fatalf("paused switch=%v", err)
	}
	if err := f.db.WithWrite(func(db *sql.DB, path string) error {
		t.Error("paused backup entered")
		return CreateBackup(db, path, time.Now())
	}); !errors.Is(err, errDatabaseUnavailable) {
		t.Fatalf("paused backup=%v", err)
	}
	assertDirectoryFixture(t, f.location, filepath.Dir(f.oldPath)+"\n")
	assertDirectoryMissing(t, f.newPath)
	assertDirectoryMissing(t, f.pendingPath())
}

func TestStage4CancelledStageWithUncertainLocationPreservesRecovery(t *testing.T) {
	f := newSwitchFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := defaultDirectorySwitchOps()
	stage := ops.stage
	ops.stage = func(l *storage.LocationSwitch, a, b, c string) error {
		if err := stage(l, a, b, c); err != nil {
			return err
		}
		// Simulate an external replacement before cancellation is handled.
		if err := os.WriteFile(f.location, []byte("external-location-change\n"), 0600); err != nil {
			return err
		}
		cancel()
		return nil
	}
	result, err := f.db.switchDirectory(ctx, f.target, f.location, f.root, ops)
	if result.Committed || !errors.Is(err, ErrDatabasePaused) || !errors.Is(err, context.Canceled) {
		t.Fatalf("uncertain cancellation=%+v, %v", result, err)
	}
	if _, err := os.Stat(f.newPath); err != nil {
		t.Fatalf("candidate lost: %v", err)
	}
	if _, err := os.Stat(f.oldPath); err != nil {
		t.Fatalf("source lost: %v", err)
	}
	if _, err := os.Stat(f.pendingPath()); err != nil {
		t.Fatalf("recovery record lost: %v", err)
	}
	assertDirectoryFixture(t, f.location, "external-location-change\n")
	if err := f.db.WithAccess(context.Background(), func(*Access) error { t.Error("uncertain database admitted work"); return nil }); !errors.Is(err, ErrDatabasePaused) {
		t.Fatalf("paused access=%v", err)
	}
}

func TestStage4OpenPanicClosesUnacceptedPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panic.db")
	var candidate *sql.DB
	marker := errors.New("startup panic")
	func() {
		defer func() {
			if p := recover(); p != marker {
				t.Fatalf("panic=%v", p)
			}
		}()
		_, _ = openSQLite(path, true, func(db *sql.DB) error {
			candidate = db
			return initializeAtomically(db, func(q *Queries) error {
				if err := initializeSchemaQueries(q); err != nil {
					return err
				}
				panic(marker)
			})
		})
	}()
	if candidate == nil {
		t.Fatal("initialization did not begin")
	}
	if _, err := candidate.Exec(`SELECT 1`); err == nil {
		t.Fatal("unaccepted pool leaked after panic")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
}
