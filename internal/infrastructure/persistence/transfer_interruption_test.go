package persistence

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type interruptedRestoreContext struct {
	context.Context
	journal string
	crash   bool
}

func (ctx interruptedRestoreContext) Err() error {
	// 实际恢复已开始写入 SQLite 回滚日志，下一批复制开始前触发中断。
	if _, err := os.Stat(ctx.journal); err == nil {
		if ctx.crash {
			os.Exit(73)
		}
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestRestoreInterruptionChild(t *testing.T) {
	target := os.Getenv("STUDYFLOW_TEST_RESTORE_TARGET")
	source := os.Getenv("STUDYFLOW_TEST_RESTORE_SOURCE")
	if target == "" || source == "" {
		t.Skip("仅由恢复中断测试启动")
	}
	db, err := OpenUnprepared(target, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := interruptedRestoreContext{
		Context: context.Background(), journal: target + "-journal", crash: true,
	}
	_, err = restoreDatabasePages(ctx, db, source)
	t.Fatalf("测试子进程应在 SQLite 开始写入后强制终止，实际返回: %v", err)
}

func TestRestoreInterruptionPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "live.db")
	source := filepath.Join(dir, "incoming.db")
	live := newRestoreFixture(t, target, "original")
	incoming := newRestoreFixture(t, source, "incoming")
	if err := incoming.WithWrite(func(db *sql.DB, _ string) error {
		// 确保恢复跨越多个 128 页批次。
		_, err := db.Exec("CREATE TABLE more_pages(data BLOB); INSERT INTO more_pages VALUES(randomblob(4194304))")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(live.Close(), incoming.Close()); err != nil {
		t.Fatal(err)
	}

	db, err := OpenUnprepared(target, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := interruptedRestoreContext{Context: context.Background(), journal: target + "-journal"}
	committed, err := restoreDatabasePages(ctx, db, source)
	if committed || !errors.Is(err, context.Canceled) {
		t.Fatalf("应在实际写入后、提交前取消: committed=%t err=%v", committed, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := restoreFixtureLabel(t, target); got != "original" {
		t.Fatalf("取消后原库不完整: %s", got)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestRestoreInterruptionChild$")
	cmd.Env = append(os.Environ(), "STUDYFLOW_TEST_RESTORE_TARGET="+target, "STUDYFLOW_TEST_RESTORE_SOURCE="+source)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 73 {
		t.Fatalf("未在写入中强制中断: %v: %s", err, output)
	}
	if got := restoreFixtureLabel(t, target); got != "original" {
		t.Fatalf("进程中断后重开未恢复原库: %s", got)
	}
}
