//go:build windows

package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"golang.org/x/sys/windows"
)

func windowsQAService(t *testing.T) (*APIService, string) {
	t.Helper()
	root := t.TempDir()
	old := filepath.Join(root, "原目录 #100%")
	if err := os.Mkdir(old, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewApp(filepath.Join(old, "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	api := &APIService{app: store, locationFile: filepath.Join(root, "storage-location.txt")}
	writeStartupFixture(t, api.locationFile, old+"\n")
	if _, err := testExec(store, `INSERT INTO subjects(name,created_at) VALUES('qa-marker','2026-11-15')`); err != nil {
		t.Fatal(err)
	}
	return api, root
}

// TestWindowsQASwitchConnectionSettings 检查切换后连接仍执行外键约束。
func TestWindowsQASwitchConnectionSettings(t *testing.T) {
	api, root := windowsQAService(t)
	var before, after int
	if err := testQueryRow(api.app, `PRAGMA foreign_keys`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := api.SetDataDirectory(filepath.Join(root, "target")); err != nil {
		t.Fatal(err)
	}
	if err := testQueryRow(api.app, `PRAGMA foreign_keys`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	t.Logf("foreign_keys before=%d after=%d", before, after)
	if after != 1 {
		t.Error("foreign key enforcement disabled after switch")
	}
	if _, err := testExec(api.app, `INSERT INTO plans(subject_id,start_time,duration_minutes) VALUES(99999,'12:00',30)`); err == nil {
		t.Error("orphan plan accepted after switch")
	}
}

// TestWindowsQAFailedSwitchRestoresUsableConnection 检查目标冲突时原连接及位置文件仍然可用。
func TestWindowsQAFailedSwitchRestoresUsableConnection(t *testing.T) {
	api, root := windowsQAService(t)
	old := api.GetDataDirectory()
	// 目标被目录占用，应在关闭原连接之前拒绝。
	block := filepath.Join(root, "target", "study.db")
	if err := os.MkdirAll(block, 0700); err != nil {
		t.Fatal(err)
	}
	if err := api.SetDataDirectory(filepath.Dir(block)); err == nil {
		t.Fatal("expected conflict")
	}
	if got := api.Call("subjects", "GET", ""); !strings.Contains(got, "qa-marker") {
		t.Fatalf("old API unusable: %s", got)
	}
	if api.GetDataDirectory() != old {
		t.Fatal("directory changed on failure")
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(old+"\n"))
}

// TestWindowsQACopyFailureRollbackSettings 通过临时目录 ACL 制造发布或快照打开失败，验证原连接保持可用。
func TestWindowsQACopyFailureRollbackSettings(t *testing.T) {
	for _, scenario := range []struct {
		name, deny string
	}{
		{name: "copy", deny: "(WD)"},
		{name: "open", deny: "(OI)(IO)(RD)"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			testWindowsQAFailureRollback(t, scenario.deny)
		})
	}
}

func testWindowsQAFailureRollback(t *testing.T, deny string) {
	t.Helper()
	api, root := windowsQAService(t)
	old := api.GetDataDirectory()
	var original *sql.DB
	if err := api.app.database.WithRead(func(db *sql.DB, _ string) error {
		original = db
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "deny-create")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := "*" + user.User.Sid.String()
	if out, err := exec.Command("icacls", target, "/deny", sid+":"+deny).CombinedOutput(); err != nil {
		t.Fatalf("set fixture ACL: %v %s", err, out)
	}
	defer func() {
		if out, err := exec.Command("icacls", target, "/remove:d", sid).CombinedOutput(); err != nil {
			t.Errorf("restore fixture ACL: %v %s", err, out)
		}
	}()
	if err := api.SetDataDirectory(target); err == nil {
		t.Fatal("expected migration failure")
	} else {
		t.Logf("migration failed as intended: %v", err)
	}
	_, statErr := os.Stat(filepath.Join(target, "study.db"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("failed preparation retained target database: %v", statErr)
	}
	if err := api.app.database.WithRead(func(db *sql.DB, _ string) error {
		if db != original {
			return fmt.Errorf("preparation failure replaced the original connection")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := api.Call("subjects", "GET", ""); !strings.Contains(got, "qa-marker") {
		t.Fatalf("rollback API unusable: %s", got)
	}
	if api.GetDataDirectory() != old {
		t.Fatal("active directory changed after copy failure")
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(old+"\n"))
	var enabled int
	if err := testQueryRow(api.app, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Errorf("rollback connection foreign_keys=%d, want 1", enabled)
	}
	if _, err := testExec(api.app, `INSERT INTO plans(subject_id,start_time,duration_minutes) VALUES(99999,'12:00',30)`); err == nil {
		t.Error("orphan plan accepted after rollback")
	}
}

// TestWindowsQAConcurrentSwitchRequestsAndBackups 检查备份、请求和目录切换共用生命周期锁。
func TestWindowsQAConcurrentSwitchRequestsAndBackups(t *testing.T) {
	api, root := windowsQAService(t)
	if _, err := testExec(api.app, `INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1')`); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 24)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			response := api.Call("subjects", "POST", fmt.Sprintf(`{"name":"concurrent-%d","color":"#123456"}`, i))
			if strings.Contains(response, `"error"`) {
				errs <- fmt.Errorf("API: %s", response)
			}
		}(i)
	}
	wg.Add(2)
	go func() { defer wg.Done(); <-start; errs <- api.SetDataDirectory(filepath.Join(root, "target")) }()
	go func() { defer wg.Done(); <-start; errs <- api.app.scheduler.BackupCheck(time.Now()) }()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	var count int
	if err := testQueryRow(api.app, `SELECT count(*) FROM subjects`).Scan(&count); err != nil || count != 21 {
		t.Errorf("subjects=%d err=%v", count, err)
	}
	entries, err := os.ReadDir(filepath.Join(api.GetDataDirectory(), "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("backups=%d", len(entries))
	}
	db, err := persistence.OpenExisting(filepath.Join(api.GetDataDirectory(), "backups", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Errorf("backup integrity=%s %v", integrity, err)
	}
}

// TestWindowsQABackupWALAndRetention 检查未 checkpoint 的 WAL、保留期限边界以及用户文件保护。
func TestWindowsQABackupWALAndRetention(t *testing.T) {
	api, _ := windowsQAService(t)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.Local)
	old := api.GetDataDirectory()
	if _, err := testExec(api.app, `PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(api.app, `PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(api.app, `INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(api.app, `INSERT INTO subjects(name,created_at) VALUES('wal-marker','2026-11-15')`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(old, "study.db-wal"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("WAL fixture: %v", err)
	}
	for _, name := range []string{"study-20261112-120000.db", "study-20261113-120000.db", "user.db", "notes.txt"} {
		writeStartupFixture(t, filepath.Join(old, "backups", name), "fixture")
	}
	if err := api.app.scheduler.BackupCheck(now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"study-20261112-120000.db", "study-20261113-120000.db"} {
		assertStartupPathMissing(t, filepath.Join(old, "backups", name))
	}
	for _, name := range []string{"user.db", "notes.txt"} {
		assertStartupFileUnchanged(t, filepath.Join(old, "backups", name), []byte("fixture"))
	}
	path := filepath.Join(old, "backups", "study-20261115-120000.db")
	db, err := persistence.OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM subjects WHERE name='wal-marker'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("WAL marker=%d %v", count, err)
	}
	// 直接调用底层备份函数只验证快照能力，产品当前没有手动备份入口。
	if err := api.app.database.WithWrite(func(db *sql.DB, path string) error { return persistence.CreateBackup(db, path, now.Add(time.Second)) }); err != nil {
		t.Fatal(err)
	}
}
