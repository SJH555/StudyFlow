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
	execSQL(t, owner, "INSERT INTO subject_versions(subject_id,effective_day,name,color,created_at,status) VALUES(7,'2026-01-01',?,'#007AFF','2026-01-01','active')", name)
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
		INSERT INTO plan_versions(plan_id,subject_id,effective_day,start_time,duration_minutes,break_minutes,weekdays,color,note,status)
		    VALUES(8,7,'2026-01-01','09:00',45,0,'1,2,3,4,5,6,0','#007AFF','','active');
		INSERT INTO sessions(id,subject_id,plan_id,start_at,end_at,duration_seconds,deleted)
		    VALUES(9,7,8,'2026-01-01T09:00:00+08:00','2026-01-01T09:30:00+08:00',1800,1);
		INSERT INTO activity_logs(id,happened_at,content) VALUES(10,'2026-01-01','读书');
		INSERT INTO stage_goals(id,title,deadline,created_at,updated_at) VALUES(11,'目标','2026-02-01','a','b');
		INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at)
		    VALUES('2026-01-01','total',50,3600,1800,'2026-01-02');
		INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at,status)
		    VALUES('2026-01-02','total',80,3600,3600,'','passed'),
		          ('2026-01-03','project',80,3600,1200,'','pending');
		INSERT INTO daily_review_items VALUES(12,'2026-01-01',7,'数学',3600,1800,'原因');
		INSERT INTO plan_versions(kind,effective_day,legacy_targets_json) VALUES('legacy_target','2026-01-01','[]');
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
	if candidate.Preview().NeedsUpgrade || candidate.Preview().Sessions != 0 {
		t.Fatalf("预览不正确: %+v", candidate.Preview())
	}
	if got := value(t, dst, "SELECT name FROM subject_versions"); got != "本机旧项目" {
		t.Fatalf("预览不应修改本机: %s", got)
	}

	var commits int
	if err := candidate.Restore(ctx, dst, func() { commits++ }); err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"SELECT status FROM daily_reviews WHERE day='2026-01-01'":                   "submitted",
		"SELECT status FROM daily_reviews WHERE day='2026-01-02'":                   "passed",
		"SELECT status FROM daily_reviews WHERE day='2026-01-03'":                   "pending",
		"SELECT name FROM subject_versions WHERE subject_id=7":                      "数学",
		"SELECT subject_id FROM plan_versions WHERE kind='recurring' AND plan_id=8": "7",
		"SELECT plan_id || ':' || deleted FROM sessions WHERE id=9":                 "8:1",
		"SELECT content FROM activity_logs WHERE id=10":                             "读书",
		"SELECT title FROM stage_goals WHERE id=11":                                 "目标",
		"SELECT threshold FROM daily_reviews":                                       "50",
		"SELECT reason FROM daily_review_items":                                     "原因",
		"SELECT legacy_targets_json FROM plan_versions WHERE kind='legacy_target'":  "[]",
		"SELECT value FROM settings WHERE key='activityStartTime'":                  "10:30",
		"SELECT value FROM settings WHERE key='futureSetting'":                      "随整表迁移",
		"SELECT count(*) FROM settings WHERE key='onlyLocal'":                       "0",
		"SELECT max(version_id) FROM goose_db_version":                              "2",
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
	src, err := persistence.NewDatabase(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { src.Close() })
	execSQL(t, src, "INSERT INTO subjects(id,name,created_at) VALUES(7,'旧版数据','2026-01-01')")
	execSQL(t, src, `INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,deleted) VALUES
		(7,'2026-01-01T09:00:00+08:00','2026-01-01T09:30:00+08:00',1800,0),
		(7,'2026-01-01T10:00:00+08:00','2026-01-01T10:30:00+08:00',1800,1)`)
	candidate, err := Prepare(ctx, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if !candidate.Preview().Legacy || !candidate.Preview().NeedsUpgrade || candidate.Preview().Sessions != 1 {
		t.Fatalf("未识别无版本旧库: %+v", candidate.Preview())
	}
	dst := database(t, filepath.Join(dir, "target.db"), "本机")
	if err := candidate.Restore(ctx, dst, func() {}); err != nil {
		t.Fatal(err)
	}
	if got := value(t, src, "SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'"); got != "0" {
		t.Fatal("源文件被修改")
	}
	if got := value(t, dst, "SELECT name FROM subject_versions"); got != "旧版数据" {
		t.Fatalf("未还原旧库数据: %s", got)
	}
}

func TestLegacyImportPreviewWithoutSessionDeleteColumn(t *testing.T) {
	source := filepath.Join(t.TempDir(), "study.db")
	db, err := persistence.OpenUnprepared(source, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL DEFAULT '#7668e8', created_at TEXT NOT NULL);
		CREATE TABLE plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL DEFAULT '1,2,3,4,5,6,0', active INTEGER NOT NULL DEFAULT 1);
		CREATE TABLE sessions (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_at TEXT NOT NULL, end_at TEXT, duration_seconds INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'timer', note TEXT NOT NULL DEFAULT '');
		CREATE TABLE activity_logs (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, content TEXT NOT NULL);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO subjects(id,name,created_at) VALUES(7,'旧版项目','2026-01-01');
		INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds)
			VALUES(7,'2026-01-01T09:00:00+08:00','2026-01-01T09:30:00+08:00',1800);
	`)
	if err := errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	candidate, err := Prepare(context.Background(), source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if preview := candidate.Preview(); !preview.Legacy || preview.Sessions != 1 {
		t.Fatalf("旧库预览不正确: %+v", preview)
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
	execSQL(t, src, "INSERT INTO goose_db_version(version_id,is_applied) VALUES(3,1)")
	if _, err := Prepare(ctx, source, time.Now()); err == nil || !strings.Contains(err.Error(), "更新版本") {
		t.Fatalf("未拒绝新版数据库: %v", err)
	}
	if err := os.WriteFile(backup, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(ctx, backup, time.Now()); err == nil {
		t.Fatal("未拒绝损坏包")
	}
	if got := value(t, dst, "SELECT name FROM subject_versions"); got != "本机原库" {
		t.Fatalf("拒绝或取消后修改了原库: %s", got)
	}
}
