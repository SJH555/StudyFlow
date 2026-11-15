package app

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func backupTestDatabase(t *testing.T, enabled string) (*sql.DB, string, time.Time, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("INSERT INTO settings(key,value) VALUES('autoBackupEnabled',?)", enabled); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)
	old := filepath.Join(dir, "study-20261107-120000.db")
	if err := os.WriteFile(old, []byte("expired backup fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	return db, path, now, old
}

func TestCheckBackupSnapshotFailureSkipsCleanup(t *testing.T) {
	for _, stage := range []string{"create", "publish"} {
		t.Run(stage, func(t *testing.T) {
			db, path, now, old := backupTestDatabase(t, "1")
			dir := filepath.Dir(old)
			name := "study-" + now.Format("20060102-150405") + ".db"
			tempName := fmt.Sprintf(".%s.%d.tmp", name, now.UnixNano())
			blocker := name
			if stage == "create" {
				blocker = tempName
			}
			blockerPath := filepath.Join(dir, blocker)
			if err := os.Mkdir(blockerPath, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(blockerPath, "keep"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := CheckBackup(db, path, now); err == nil {
				t.Fatalf("expected snapshot %s failure", stage)
			}
			if _, err := os.Stat(old); err != nil {
				t.Fatalf("expired backup should remain after snapshot failure: %v", err)
			}
			if stage == "publish" {
				if _, err := os.Stat(filepath.Join(dir, tempName)); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("temporary snapshot should be cleaned after publish failure: %v", err)
				}
			}
		})
	}
}

func TestCheckBackupReturnsCleanupErrorWhileDisabled(t *testing.T) {
	db, path, now, old := backupTestDatabase(t, "0")
	dir := filepath.Dir(old)
	probe := filepath.Join(dir, "permission-probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Skipf("cannot set directory permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if err := os.Remove(probe); !errors.Is(err, fs.ErrPermission) {
		t.Skipf("directory permissions do not block deletion on this platform: %v", err)
	}
	if err := CheckBackup(db, path, now); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("expected cleanup permission error, got %v", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("backup should remain after failed removal: %v", err)
	}
}
