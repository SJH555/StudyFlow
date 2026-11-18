//go:build linux && !android

package persistence

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"modernc.org/sqlite"
)

// The limit applies only to this disposable child process, never the host disk.
func TestStage5BoundedFileWriteFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestStage5FileLimitChild$", "-test.v", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "STUDYFLOW_STAGE5_FILE_LIMIT_ROOT="+t.TempDir())
	output, err := cmd.CombinedOutput()
	t.Logf("bounded OS write test:\n%s", output)
	if err != nil {
		t.Fatalf("file-limit child: %v", err)
	}
}

func TestStage5FileLimitChild(t *testing.T) {
	root := os.Getenv("STUDYFLOW_STAGE5_FILE_LIMIT_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	path := filepath.Join(root, "limited.db")
	d, err := NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.db.Exec(`CREATE TABLE io_probe(id INTEGER PRIMARY KEY,value BLOB);INSERT INTO io_probe VALUES(1,'committed baseline');PRAGMA cache_size=2;PRAGMA cache_spill=ON`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	limit.Cur = uint64(info.Size() + 8192)
	signal.Ignore(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = d.WithAccess(context.Background(), func(a *Access) error {
		return a.Queries(context.Background()).WithTx(func(q *Queries) error {
			calls++
			if _, err := q.Exec(`UPDATE io_probe SET value='must rollback' WHERE id=1`); err != nil {
				return err
			}
			_, err := q.Exec(`INSERT INTO io_probe VALUES(2,zeroblob(1048576))`)
			return err
		})
	})
	var native *sqlite.Error
	if !errors.As(err, &native) || (native.Code()&255 != 10 && native.Code()&255 != 13) {
		t.Fatalf("expected actual OS write failure: %v", err)
	}
	t.Logf("native SQLite code=%d, callbacks=%d, paused=%t, error=%v", native.Code(), calls, d.pauseErr != nil, err)
	if calls != 1 {
		t.Fatal("business callback retried")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var value string
	var count int
	if err := reopened.QueryRow(`SELECT value FROM io_probe WHERE id=1`).Scan(&value); err != nil || value != "committed baseline" {
		t.Fatalf("partial earlier update=%s, %v", value, err)
	}
	if err := reopened.QueryRow(`SELECT count(*) FROM io_probe`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("partial inserted rows=%d, %v", count, err)
	}
	requireStage5Integrity(t, reopened)
}
