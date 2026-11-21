package persistence

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreVerifiedSnapshotRestoresOriginalOnVerificationFailure(t *testing.T) {
	ctx := context.Background()
	for _, failRecovery := range []bool{false, true} {
		name := "recover"
		if failRecovery {
			name = "quarantine"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			db := newRestoreFixture(t, filepath.Join(dir, "live.db"), "original")
			source := filepath.Join(dir, "incoming.db")
			incoming := newRestoreFixture(t, source, "incoming")
			if err := incoming.Close(); err != nil {
				t.Fatal(err)
			}
			rollback := filepath.Join(dir, "rollback.db")
			fault := errors.New("injected post-commit validation failure")
			verify := func(exec Executor) error {
				var value string
				if err := exec.QueryRowContext(ctx, "SELECT label FROM restore_probe").Scan(&value); err != nil {
					return err
				}
				if value == "incoming" {
					return fault
				}
				return nil
			}
			calls := 0
			faultOnRecovery := name == "quarantine"
			err := db.RestoreVerifiedSnapshot(ctx, source, rollback, func(exec Executor) error {
				calls++
				if calls == 3 && faultOnRecovery {
					return errors.New("injected rollback validation failure")
				}
				return verify(exec)
			}, func(Executor) error { return nil }, func() { t.Fatal("校验失败不应发布") })
			if !errors.Is(err, fault) || calls != 3 {
				t.Fatalf("未处理提交后的校验失败: calls=%d err=%v", calls, err)
			}
			if faultOnRecovery {
				if !errors.Is(err, ErrRestoreRecoveryUncertain) || db.WithRead(func(*sql.DB, string) error { return nil }) == nil {
					t.Fatalf("不确定结果必须隔离: %v", err)
				}
			} else if got := restoreFixtureLabel(t, filepath.Join(dir, "live.db")); got != "original" {
				t.Fatalf("恢复后原库值错误: %s", got)
			}
			if got := restoreFixtureLabel(t, rollback); got != "original" {
				t.Fatalf("回滚副本必须可打开: %s", got)
			}
		})
	}
}

func TestRestoreVerifiedSnapshotPreservesOriginalOnBackupAndSourceFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	current := filepath.Join(dir, "live.db")
	db := newRestoreFixture(t, current, "original")
	source := filepath.Join(dir, "incoming.db")
	if err := os.WriteFile(source, []byte("corrupt database"), 0600); err != nil {
		t.Fatal(err)
	}
	rollback := filepath.Join(dir, "rollback.db")
	if err := os.WriteFile(rollback, []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	verify := func(Executor) error { return nil }
	commit := func() { t.Fatal("失败不应发布") }
	err := db.RestoreVerifiedSnapshot(ctx, source, rollback, verify, verify, commit)
	if err == nil || !strings.Contains(err.Error(), "回滚副本") {
		t.Fatalf("备份失败应停止: %v", err)
	}
	if old, _ := os.ReadFile(rollback); string(old) != "existing file" {
		t.Fatal("不应覆盖已有文件")
	}
	err = db.RestoreVerifiedSnapshot(ctx, source, filepath.Join(dir, "owned.db"), verify, verify, commit)
	if err == nil || !strings.Contains(err.Error(), "原库保留") {
		t.Fatalf("复制失败应保留原库: %v", err)
	}
	if label := restoreFixtureLabel(t, current); label != "original" {
		t.Fatalf("失败修改了原库: %s", label)
	}
}

func newRestoreFixture(t *testing.T, path, label string) *Database {
	t.Helper()
	db, err := NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.WithWrite(func(db *sql.DB, _ string) error {
		_, err := db.Exec("CREATE TABLE restore_probe(label TEXT); INSERT INTO restore_probe VALUES(?)", label)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return db
}

func restoreFixtureLabel(t *testing.T, path string) string {
	t.Helper()
	db, err := OpenUnprepared(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var label string
	if err := db.QueryRow("SELECT label FROM restore_probe").Scan(&label); err != nil {
		t.Fatal(err)
	}
	return label
}
