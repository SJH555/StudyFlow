//go:build linux && !android

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func TestStage5BackupOSWriteFailurePreservesOldBackup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestStage5BackupFileLimitChild$", "-test.v", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "STUDYFLOW_STAGE5_BACKUP_LIMIT_ROOT="+t.TempDir())
	output, err := cmd.CombinedOutput()
	t.Logf("bounded backup write test:\n%s", output)
	if err != nil {
		t.Fatalf("backup child=%v", err)
	}
}
func TestStage5BackupFileLimitChild(t *testing.T) {
	root := os.Getenv("STUDYFLOW_STAGE5_BACKUP_LIMIT_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	store, err := NewApp(filepath.Join(root, "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `CREATE TABLE large_payload(value BLOB);INSERT INTO large_payload VALUES(zeroblob(1048576));INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1') ON CONFLICT(key) DO UPDATE SET value='1'`); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "backups", "study-20200101-000000.db")
	writeStartupFixture(t, old, "old backup must remain")
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	limit.Cur = 65536
	signal.Ignore(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	err = store.scheduler.BackupCheck(time.Now())
	var native *sqlite.Error
	if !errors.As(err, &native) || (native.Code()&255 != 10 && native.Code()&255 != 13) {
		t.Fatalf("expected actual backup write error, got %v", err)
	}
	t.Logf("native backup error code=%d: %v", native.Code(), err)
	assertStartupFileUnchanged(t, old, []byte("old backup must remain"))
	entries, err := os.ReadDir(filepath.Dir(old))
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial backup published or temporary file leaked: %v, %v", entries, err)
	}
	var size int
	if err := testQueryRow(store, `SELECT length(value) FROM large_payload`).Scan(&size); err != nil || size != 1048576 {
		t.Fatalf("source data changed: %d, %v", size, err)
	}
}
