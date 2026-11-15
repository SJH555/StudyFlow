//go:build windows && database_audit

package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestWindowsQASwitchPreservesResources 以验收要求检查旧数据库、备份和用户文件是否保留。
func TestWindowsQASwitchPreservesResources(t *testing.T) {
	api, root := windowsQAService(t)
	old := api.GetDataDirectory()
	writeStartupFixture(t, filepath.Join(old, "backups", "study-20261115-100000.db"), "backup")
	writeStartupFixture(t, filepath.Join(old, "backups", "user-notes.txt"), "keep")
	writeStartupFixture(t, filepath.Join(old, "user.txt"), "keep")
	if err := api.SetDataDirectory(filepath.Join(root, "新目录 #100%")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"study.db", "backups/study-20261115-100000.db", "backups/user-notes.txt", "user.txt"} {
		if _, err := os.Stat(filepath.Join(old, filepath.FromSlash(name))); err != nil {
			t.Errorf("old resource lost: %s: %v", name, err)
		}
	}
}

// TestWindowsQASwitchWithExternalWALConnection 检查其他连接仍打开时切换能否保留已提交的 WAL 数据。
func TestWindowsQASwitchWithExternalWALConnection(t *testing.T) {
	api, root := windowsQAService(t)
	oldPath := filepath.Join(api.GetDataDirectory(), "study.db")
	if _, err := testExec(api.app, `PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(api.app, `PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	external, err := sql.Open("sqlite", oldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	var count int
	if err := external.QueryRow(`SELECT count(*) FROM subjects`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(api.app, `INSERT INTO subjects(name,created_at) VALUES('committed-in-wal','2026-11-15')`); err != nil {
		t.Fatal(err)
	}
	if err := api.SetDataDirectory(filepath.Join(root, "target")); err != nil {
		t.Fatal(err)
	}
	if err := testQueryRow(api.app, `SELECT count(*) FROM subjects WHERE name='committed-in-wal'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed WAL row after switch=%d, err=%v", count, err)
	}
}
