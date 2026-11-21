package transfer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/upgrade"
)

func database(t *testing.T, path, name string) *persistence.Database {
	t.Helper()
	ctx := context.Background()
	db, err := persistence.OpenUnprepared(path, true)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := upgrade.New()
	if err != nil {
		t.Fatal(err)
	}
	err = persistence.WithStartupAccess(ctx, db, func(a *persistence.Access) error {
		state, err := manager.Inspect(ctx, a)
		if err != nil {
			return err
		}
		return manager.Apply(ctx, a, state)
	})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	owner, err := persistence.AdoptDatabase(db, path)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close() })
	execSQL(t, owner, "INSERT INTO subjects(id,name,created_at) VALUES(7,?,'2026-01-01')", name)
	return owner
}

func execSQL(t *testing.T, db *persistence.Database, sqlText string, args ...any) {
	t.Helper()
	err := db.WithWrite(func(exec *sql.DB, _ string) error {
		_, err := exec.Exec(sqlText, args...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func value(t *testing.T, db *persistence.Database, sqlText string) string {
	t.Helper()
	var got string
	err := db.WithRead(func(exec *sql.DB, _ string) error {
		return exec.QueryRow(sqlText).Scan(&got)
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTransferRestoresWholeDatabaseAndKeepsOneLiveCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src := database(t, filepath.Join(dir, "source.db"), "数学")
	dst := database(t, filepath.Join(dir, "target.db"), "本机旧项目")
	execSQL(t, src, `
		INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(8,7,'09:00',45);
		INSERT INTO sessions(id,subject_id,plan_id,start_at,end_at,duration_seconds,deleted)
		    VALUES(9,7,8,'2026-01-01T09:00:00+08:00','2026-01-01T09:30:00+08:00',1800,1);
		INSERT INTO activity_logs(id,happened_at,content) VALUES(10,'2026-01-01','读书');
		INSERT INTO stage_goals(id,title,deadline,created_at,updated_at) VALUES(11,'目标','2026-02-01','a','b');
		INSERT INTO daily_reviews VALUES('2026-01-01','total',50,3600,1800,'2026-01-02');
		INSERT INTO daily_review_items VALUES(12,'2026-01-01',7,'数学',3600,1800,'原因');
		INSERT INTO daily_plan_targets VALUES('2026-01-01','[]');
		INSERT INTO settings VALUES('activityStartTime','10:30'),('futureSetting','随整表迁移');
	`)
	execSQL(t, dst, "INSERT INTO settings VALUES('activityStartTime','09:00'),('onlyLocal','旧值')")
	backup := filepath.Join(dir, "出库"+Extension)
	if err := Export(ctx, src, backup, time.Now()); err != nil {
		t.Fatal(err)
	}
	candidate, err := Prepare(ctx, backup, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if candidate.Preview().NeedsUpgrade || candidate.Preview().Sessions != 1 {
		t.Fatalf("预览不正确: %+v", candidate.Preview())
	}
	if got := value(t, dst, "SELECT name FROM subjects"); got != "本机旧项目" {
		t.Fatalf("预览不应修改本机: %s", got)
	}

	var commits int
	if err := candidate.Restore(ctx, dst, func() { commits++ }); err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"SELECT name FROM subjects WHERE id=7":                      "数学",
		"SELECT subject_id FROM plans WHERE id=8":                   "7",
		"SELECT plan_id || ':' || deleted FROM sessions WHERE id=9": "8:1",
		"SELECT content FROM activity_logs WHERE id=10":             "读书",
		"SELECT title FROM stage_goals WHERE id=11":                 "目标",
		"SELECT threshold FROM daily_reviews":                       "50",
		"SELECT reason FROM daily_review_items":                     "原因",
		"SELECT targets_json FROM daily_plan_targets":               "[]",
		"SELECT value FROM settings WHERE key='activityStartTime'":  "10:30",
		"SELECT value FROM settings WHERE key='futureSetting'":      "随整表迁移",
		"SELECT count(*) FROM settings WHERE key='onlyLocal'":       "0",
		"SELECT max(version_id) FROM goose_db_version":              "1",
	}
	for query, want := range checks {
		if got := value(t, dst, query); got != want {
			t.Errorf("%s = %s, want %s", query, got, want)
		}
	}
	if commits != 1 || dst.Path() != filepath.Join(dir, "target.db") {
		t.Fatalf("必须只提交一次，保留本机工作库路径: commits=%d path=%s", commits, dst.Path())
	}
	if files, _ := filepath.Glob(filepath.Join(dir, ".studyflow-rollback-*")); len(files) != 0 {
		t.Fatalf("成功后不应保留旧库: %v", files)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("不得删除用户选择的备份包: %v", err)
	}
}

func TestLegacyImportUpgradesCopyOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "study.db")
	src := database(t, source, "旧版数据")
	execSQL(t, src, "DROP TABLE goose_db_version")
	candidate, err := Prepare(ctx, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if !candidate.Preview().Legacy || !candidate.Preview().NeedsUpgrade {
		t.Fatalf("未识别无版本旧库: %+v", candidate.Preview())
	}
	dst := database(t, filepath.Join(dir, "target.db"), "本机")
	if err := candidate.Restore(ctx, dst, func() {}); err != nil {
		t.Fatal(err)
	}
	if got := value(t, src, "SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'"); got != "0" {
		t.Fatal("源文件被修改")
	}
	if got := value(t, dst, "SELECT name FROM subjects"); got != "旧版数据" {
		t.Fatalf("未还原旧库数据: %s", got)
	}
}

func TestTransferRejectsActiveDamagedNewerAndCancelled(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	src := database(t, source, "来源")
	dst := database(t, filepath.Join(dir, "target.db"), "本机原库")
	const running = "INSERT INTO sessions(subject_id,start_at,paused_at) VALUES(7,'2026-01-01','2026-01-01')"
	execSQL(t, src, running)
	backup := filepath.Join(dir, "copy"+Extension)
	if err := Export(ctx, src, backup, time.Now()); err == nil || !strings.Contains(err.Error(), "未结束") {
		t.Fatalf("未阻止导出暂停的计时: %v", err)
	}
	if _, err := Prepare(ctx, source, time.Now()); err == nil || !strings.Contains(err.Error(), "未结束") {
		t.Fatalf("未阻止导入暂停的计时: %v", err)
	}
	execSQL(t, src, "DELETE FROM sessions")
	if err := Export(ctx, src, backup, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Export(ctx, src, backup, time.Now()); err == nil {
		t.Fatal("已有备份不应被覆盖")
	}
	candidate, err := Prepare(ctx, backup, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	execSQL(t, dst, running)
	if err := candidate.Restore(ctx, dst, func() { t.Fatal("不应提交") }); err == nil || !strings.Contains(err.Error(), "未结束") {
		t.Fatalf("未复查本机计时状态: %v", err)
	}
	execSQL(t, dst, "DELETE FROM sessions")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := candidate.Restore(cancelled, dst, func() { t.Fatal("不应提交") }); !errors.Is(err, context.Canceled) {
		t.Fatalf("未响应取消: %v", err)
	}
	execSQL(t, src, "INSERT INTO goose_db_version(version_id,is_applied) VALUES(2,1)")
	if _, err := Prepare(ctx, source, time.Now()); err == nil || !strings.Contains(err.Error(), "更新版本") {
		t.Fatalf("未拒绝新版数据库: %v", err)
	}
	if err := os.WriteFile(backup, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(ctx, backup, time.Now()); err == nil {
		t.Fatal("未拒绝损坏包")
	}
	if got := value(t, dst, "SELECT name FROM subjects"); got != "本机原库" {
		t.Fatalf("拒绝或取消后修改了原库: %s", got)
	}
}
