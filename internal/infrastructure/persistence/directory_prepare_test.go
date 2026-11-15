package persistence

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func directoryFixture(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "old", "study.db")
	if err := os.Mkdir(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	location := filepath.Join(root, "storage-location.txt")
	writeDirectoryFixture(t, location, filepath.Dir(source)+"\n")
	return db, source, location
}

func writeDirectoryFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertDirectoryFixture(t *testing.T, path, content string) {
	t.Helper()
	if actual, err := os.ReadFile(path); err != nil || string(actual) != content {
		t.Fatalf("%s = %q, %v; want %q", path, actual, err, content)
	}
}

func assertDirectoryMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s should be absent: %v", path, err)
	}
}

func TestPrepareDirectorySnapshotIncludesWALAndPreservesRowID(t *testing.T) {
	db, source, location := directoryFixture(t)
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode = %q, %v", mode, err)
	}
	for _, statement := range []string{
		`PRAGMA wal_autocheckpoint=0`,
		`DELETE FROM settings WHERE key = 'dailyReviewEnabled'`,
		`INSERT INTO settings(rowid, key, value) VALUES (41, 'only-in-wal', 'committed')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Stat(source + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("no WAL fixture: %v, %v", info, err)
	}
	oldBackup := filepath.Join(filepath.Dir(source), "backups", "study-test.db")
	writeDirectoryFixture(t, oldBackup, "backup")
	target := filepath.Join(filepath.Dir(location), "new", "space % # & 中文")
	prepared, err := prepareDirectory(context.Background(), db, source, target, location)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.sameDirectory || prepared.databasePath != filepath.Join(target, "study.db") {
		t.Fatalf("prepared path = %q, same directory = %t", prepared.databasePath, prepared.sameDirectory)
	}
	candidate, err := OpenExisting(prepared.databasePath)
	if err != nil {
		t.Fatal(err)
	}
	var rowID, defaults int
	var value string
	if err := candidate.QueryRow(`SELECT rowid, value FROM settings WHERE key='only-in-wal'`).Scan(&rowID, &value); err != nil || rowID != 41 || value != "committed" {
		t.Errorf("snapshot row = %d, %q, %v", rowID, value, err)
	}
	if err := candidate.QueryRow(`SELECT count(*) FROM settings WHERE key='dailyReviewEnabled'`).Scan(&defaults); err != nil || defaults != 0 {
		t.Errorf("startup defaults appeared in snapshot: %d, %v", defaults, err)
	}
	if err := candidate.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE settings SET value='source still open' WHERE key='only-in-wal'`); err != nil {
		t.Fatal(err)
	}
	assertDirectoryFixture(t, oldBackup, "backup")
	assertDirectoryFixture(t, filepath.Join(target, "backups", filepath.Base(oldBackup)), "backup")
	assertDirectoryFixture(t, location, filepath.Dir(source)+"\n")
	if err := prepared.cleanup(); err != nil {
		t.Fatal(err)
	}
	assertDirectoryMissing(t, filepath.Dir(target))
}

func TestPrepareDirectoryBackupConflictRollsBackOnlyOwnedResources(t *testing.T) {
	db, source, location := directoryFixture(t)
	oldBackups := filepath.Join(filepath.Dir(source), "backups")
	target := filepath.Join(filepath.Dir(location), "new")
	targetBackups := filepath.Join(target, "backups")
	writeDirectoryFixture(t, filepath.Join(oldBackups, "a.db"), "first backup")
	writeDirectoryFixture(t, filepath.Join(oldBackups, "z.db"), "last backup")
	writeDirectoryFixture(t, filepath.Join(targetBackups, "z.db"), "user database")
	writeDirectoryFixture(t, filepath.Join(targetBackups, "keep", "note.txt"), "user note")
	prepared, err := prepareDirectory(context.Background(), db, source, target, location)
	if prepared != nil || err == nil || !strings.Contains(err.Error(), "z.db") {
		t.Fatalf("expected backup collision, got %v, %v", prepared, err)
	}
	assertDirectoryMissing(t, filepath.Join(target, "study.db"))
	assertDirectoryMissing(t, filepath.Join(targetBackups, "a.db"))
	assertDirectoryFixture(t, filepath.Join(targetBackups, "z.db"), "user database")
	assertDirectoryFixture(t, filepath.Join(targetBackups, "keep", "note.txt"), "user note")
	assertDirectoryFixture(t, filepath.Join(oldBackups, "a.db"), "first backup")
	assertDirectoryFixture(t, filepath.Join(oldBackups, "z.db"), "last backup")
	assertDirectoryFixture(t, location, filepath.Dir(source)+"\n")
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(targetBackups)
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected backup remnants: %v, %v", entries, err)
	}
}

func TestPrepareDirectoryRecoversFromBackupIOFailures(t *testing.T) {
	for _, failure := range []string{"partial copy", "source changed", "file sync", "directory sync"} {
		t.Run(failure, func(t *testing.T) {
			db, source, location := directoryFixture(t)
			target := filepath.Join(filepath.Dir(location), "new")
			oldBackups := filepath.Join(filepath.Dir(source), "backups")
			lastBackup := filepath.Join(oldBackups, "z.db")
			userFile := filepath.Join(target, "backups", "user.txt")
			writeDirectoryFixture(t, filepath.Join(oldBackups, "a.db"), "first backup")
			writeDirectoryFixture(t, lastBackup, "last backup")
			writeDirectoryFixture(t, userFile, "user file")
			injected := errors.New("injected filesystem failure")
			ops := directoryPrepareIO{
				copyBackup:    io.Copy,
				syncFile:      (*os.File).Sync,
				syncDirectory: syncPreparedDirectory,
			}
			switch failure {
			case "file sync":
				var syncCalls int
				ops.syncFile = func(file *os.File) error {
					syncCalls++
					if syncCalls == 1 {
						return file.Sync()
					}
					assertDirectoryFixture(t, filepath.Join(target, "backups", "a.db"), "first backup")
					assertDirectoryMissing(t, filepath.Join(target, "backups", "z.db"))
					if info, err := file.Stat(); err != nil || info.Size() != int64(len("last backup")) {
						t.Fatalf("staged backup was not copied before sync: %v, %v", info, err)
					}
					return injected
				}
			case "directory sync":
				ops.syncDirectory = func(path string) error {
					if filepath.Base(path) != filepath.Base(target) {
						return syncPreparedDirectory(path)
					}
					assertDirectoryFixture(t, filepath.Join(target, "backups", "z.db"), "last backup")
					return injected
				}
			default:
				ops.copyBackup = func(dst io.Writer, src io.Reader) (int64, error) {
					if filepath.Base(src.(*os.File).Name()) != filepath.Base(lastBackup) {
						return io.Copy(dst, src)
					}
					assertDirectoryFixture(t, filepath.Join(target, "backups", "a.db"), "first backup")
					copied, err := io.CopyN(dst, src, 1)
					if err != nil {
						return copied, err
					}
					if failure == "partial copy" {
						return copied, injected
					}
					writeDirectoryFixture(t, lastBackup, "last backup changed mid-copy")
					rest, err := io.Copy(dst, src)
					return copied + rest, err
				}
			}
			prepared, err := prepareDirectoryWithIO(context.Background(), db, source, target, location, ops)
			if prepared != nil || err == nil {
				t.Fatalf("expected failure, got %v, %v", prepared, err)
			}
			if failure == "source changed" {
				if !strings.Contains(err.Error(), "备份文件在复制期间发生变化") {
					t.Fatalf("expected source change error, got %v", err)
				}
			} else if !errors.Is(err, injected) {
				t.Fatalf("expected injected failure, got %v", err)
			}
			assertDirectoryMissing(t, filepath.Join(target, "study.db"))
			assertDirectoryFixture(t, userFile, "user file")
			assertDirectoryFixture(t, location, filepath.Dir(source)+"\n")
			entries, err := os.ReadDir(filepath.Dir(userFile))
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected backup remnants: %v, %v", entries, err)
			}
			if err := db.Ping(); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO settings(key, value) VALUES ('backup-io-test', 'old database writable')`); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPrepareDirectoryRejectsBackupFIFOWithoutWaitingForWriter(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("requires a Unix FIFO")
	}
	db, source, location := directoryFixture(t)
	oldBackups := filepath.Join(filepath.Dir(source), "backups")
	if err := os.Mkdir(oldBackups, 0700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(oldBackups, "pipe.db")
	if output, err := exec.Command(mkfifo, fifo).CombinedOutput(); err != nil {
		t.Fatalf("create FIFO: %v, %s", err, output)
	}
	target := filepath.Join(filepath.Dir(location), "new")
	if prepared, err := prepareDirectory(context.Background(), db, source, target, location); prepared != nil || err == nil || !strings.Contains(err.Error(), "备份源不是普通文件") {
		t.Fatalf("expected FIFO to be rejected, got %v, %v", prepared, err)
	}
	assertDirectoryMissing(t, target)
	if info, err := os.Lstat(fifo); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("source FIFO should remain in place: %v, %v", info, err)
	}
}

func TestPrepareDirectoryKeepsLegacyBackupSelection(t *testing.T) {
	db, source, location := directoryFixture(t)
	oldBackups := filepath.Join(filepath.Dir(source), "backups")
	writeDirectoryFixture(t, filepath.Join(oldBackups, "keep.db"), "backup")
	writeDirectoryFixture(t, filepath.Join(oldBackups, "skip.DB"), "uppercase suffix")
	writeDirectoryFixture(t, filepath.Join(oldBackups, "skip.txt"), "notes")
	writeDirectoryFixture(t, filepath.Join(oldBackups, "directory.db", "nested.db"), "nested backup")
	link := filepath.Join(oldBackups, "link.db")
	linkErr := os.Symlink("keep.db", link)
	if linkErr != nil && runtime.GOOS != "windows" {
		t.Fatal(linkErr)
	}
	target := filepath.Join(filepath.Dir(location), "new")
	if _, err := prepareDirectory(context.Background(), db, source, target, location); err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(target, "backups")
	assertDirectoryFixture(t, filepath.Join(backups, "keep.db"), "backup")
	for _, name := range []string{"skip.DB", "skip.txt", "directory.db"} {
		assertDirectoryMissing(t, filepath.Join(backups, name))
	}
	if linkErr == nil {
		path := filepath.Join(backups, "link.db")
		assertDirectoryFixture(t, path, "backup")
		if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("copied symlink should be a regular file: %v, %v", info, err)
		}
	}
}

func TestPrepareDirectorySnapshotFailureAndCleanup(t *testing.T) {
	db, source, location := directoryFixture(t)
	target := filepath.Join(filepath.Dir(location), "new", "data")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareDirectory(ctx, db, source, target, location); !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot failure = %v", err)
	}
	assertDirectoryMissing(t, filepath.Dir(target))
	assertDirectoryFixture(t, location, filepath.Dir(source)+"\n")
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareDirectoryReleasesFailedSQLiteBackup(t *testing.T) {
	db, source, location := directoryFixture(t)
	target := filepath.Join(filepath.Dir(location), "new")
	// SQLite 无法在写入期间从同一连接复制。将事务保留在 snapshotDatabase 使用的唯一池化连接上。
	if _, err := db.Exec(`BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key, value) VALUES ('snapshot-test', 'uncommitted')`); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDirectory(context.Background(), db, source, target, location); err == nil || !strings.Contains(err.Error(), "创建数据库快照") {
		t.Fatalf("expected SQLite backup failure, got %v", err)
	}
	assertDirectoryMissing(t, target)
	if _, err := db.Exec(`ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareDirectory(context.Background(), db, source, target, location)
	if err != nil {
		t.Fatalf("snapshot retry failed: %v", err)
	}
	if err := prepared.cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedDirectoryCleanupRetainsReplacementAndUserFiles(t *testing.T) {
	db, source, location := directoryFixture(t)
	target := filepath.Join(filepath.Dir(location), "new")
	writeDirectoryFixture(t, filepath.Join(filepath.Dir(source), "backups", "archive.db"), "backup")
	writeDirectoryFixture(t, filepath.Join(target, "backups", "user.db"), "existing user data")
	prepared, err := prepareDirectory(context.Background(), db, source, target, location)
	if err != nil {
		t.Fatal(err)
	}
	// 保留原文件存活，使其文件身份无法被复用。
	snapshot := prepared.databasePath + ".moved"
	if err := os.Rename(prepared.databasePath, snapshot); err != nil {
		t.Fatal(err)
	}
	writeDirectoryFixture(t, prepared.databasePath, "replacement")
	if err := prepared.cleanup(); err == nil || !strings.Contains(err.Error(), prepared.databasePath) {
		t.Fatalf("cleanup should report identity mismatch, got %v", err)
	}
	assertDirectoryFixture(t, prepared.databasePath, "replacement")
	assertDirectoryFixture(t, filepath.Join(target, "backups", "user.db"), "existing user data")
	assertDirectoryMissing(t, filepath.Join(target, "backups", "archive.db"))
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedDirectoryCleanupDoesNotRemovePopulatedDirectories(t *testing.T) {
	db, source, location := directoryFixture(t)
	target := filepath.Join(filepath.Dir(location), "new", "data")
	prepared, err := prepareDirectory(context.Background(), db, source, target, location)
	if err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(target, "user.txt")
	writeDirectoryFixture(t, userFile, "user file added during preparation")
	if err := prepared.cleanup(); err == nil || !strings.Contains(err.Error(), target) {
		t.Fatalf("cleanup should report retained nonempty directories, got %v", err)
	}
	assertDirectoryFixture(t, userFile, "user file added during preparation")
	assertDirectoryMissing(t, prepared.databasePath)
}

func TestDirectoryPreparationPathRisks(t *testing.T) {
	db, source, location := directoryFixture(t)
	root, old := filepath.Dir(location), filepath.Dir(source)
	backups := filepath.Join(old, "backups")
	writeDirectoryFixture(t, filepath.Join(backups, "old.db"), "old backup")
	occupied := filepath.Join(root, "occupied")
	writeDirectoryFixture(t, filepath.Join(occupied, "study.db"), "user data")
	sidecar := filepath.Join(root, "sidecar")
	writeDirectoryFixture(t, filepath.Join(sidecar, "study.db-wal"), "user sidecar")
	for _, target := range []string{occupied, sidecar, backups, filepath.Join(backups, "new", "nested"), root + "\nother"} {
		if prepared, err := prepareDirectory(context.Background(), db, source, target, location); err == nil || prepared != nil {
			t.Errorf("unsafe target %q = %v, %v", target, prepared, err)
		}
	}
	assertDirectoryMissing(t, filepath.Join(backups, "new"))
	assertDirectoryFixture(t, filepath.Join(occupied, "study.db"), "user data")
	assertDirectoryFixture(t, filepath.Join(sidecar, "study.db-wal"), "user sidecar")
	prepared, err := prepareDirectory(context.Background(), db, source, old, location)
	if err != nil || !prepared.sameDirectory || len(prepared.resources) != 0 {
		t.Fatalf("same directory should be a no-op: %v, %v", prepared, err)
	}
	t.Run("symlink aliases", func(t *testing.T) {
		alias := filepath.Join(root, "alias")
		if err := os.Symlink(old, alias); err != nil {
			t.Skipf("directory symlinks unavailable: %v", err)
		}
		if prepared, err := prepareDirectory(context.Background(), db, source, alias, location); err != nil || !prepared.sameDirectory {
			t.Fatalf("source alias should be a no-op: %v, %v", prepared, err)
		}
		if _, err := prepareDirectory(context.Background(), db, source, filepath.Join(alias, "backups", "new"), location); err == nil {
			t.Fatal("target inside backup alias was accepted")
		}
		target := filepath.Join(root, "linked-backups")
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(backups, filepath.Join(target, "backups")); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareDirectory(context.Background(), db, source, target, location); err == nil {
			t.Fatal("backup directory alias was accepted")
		}
		assertDirectoryMissing(t, filepath.Join(target, "study.db"))
	})
}

func TestDirectoryPreparationPreservesPathWhitespace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows normalizes trailing spaces in ordinary Win32 paths")
	}
	db, source, location := directoryFixture(t)
	if _, err := prepareDirectory(context.Background(), db, source, " \t ", location); err == nil {
		t.Fatal("whitespace-only path was accepted")
	}
	target := filepath.Join(filepath.Dir(location), " leading and trailing ")
	prepared, err := prepareDirectory(context.Background(), db, source, target, location)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.databasePath != filepath.Join(target, "study.db") {
		t.Fatalf("path was rewritten: %q", prepared.databasePath)
	}
	assertDirectoryMissing(t, strings.TrimSpace(target))
}

func TestDirectoryPreparationWindowsAliases(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows filesystem semantics")
	}
	db, source, location := directoryFixture(t)
	root, old := filepath.Dir(location), filepath.Dir(source)
	backups := filepath.Join(old, "backups")
	writeDirectoryFixture(t, filepath.Join(backups, "original.db"), "backup")
	t.Run("case alias", func(t *testing.T) {
		alias := filepath.Join(root, "OLD")
		if _, err := os.Stat(alias); os.IsNotExist(err) {
			t.Skip("case-sensitive test directory")
		} else if err != nil {
			t.Fatal(err)
		}
		prepared, err := prepareDirectory(context.Background(), db, source, alias, location)
		if err != nil || !prepared.sameDirectory {
			t.Fatalf("case alias should be a no-op: %v, %v", prepared, err)
		}
	})
	t.Run("junction to backups", func(t *testing.T) {
		alias := filepath.Join(root, "junction")
		if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", alias, backups).CombinedOutput(); err != nil {
			t.Fatalf("create junction: %v, %s", err, output)
		}
		t.Cleanup(func() { _ = os.Remove(alias) })
		if _, err := prepareDirectory(context.Background(), db, source, filepath.Join(alias, "new"), location); err == nil {
			t.Fatal("target inside backup junction was accepted")
		}
		assertDirectoryMissing(t, filepath.Join(backups, "new"))
	})
}
