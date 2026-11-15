//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsBindingKeepsDataDirectoryBehavior(t *testing.T) {
	api := newAPITestService(t)
	root := t.TempDir()
	api.locationFile = filepath.Join(root, "storage-location.txt")
	old := api.GetDataDirectory()
	writeStartupFixture(t, api.locationFile, old+"\n")
	target := filepath.Join(root, "target")
	if err := api.SetDataDirectory("  " + target + "  "); err != nil {
		t.Fatal(err)
	}
	if got := api.GetDataDirectory(); got != target {
		t.Fatalf("directory = %q, want %q", got, target)
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(target+"\n"))
	assertStartupPathMissing(t, filepath.Join(old, "study.db"))

	conflict := filepath.Join(root, "conflict")
	writeStartupFixture(t, filepath.Join(conflict, "study.db"), "existing")
	if err := api.SetDataDirectory(conflict); err == nil || !strings.Contains(err.Error(), "目标目录中已存在 study.db") {
		t.Fatalf("legacy conflict response = %v", err)
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(target+"\n"))
	assertStartupFileUnchanged(t, filepath.Join(conflict, "study.db"), []byte("existing"))
}

func TestChangingDataDirectoryMovesBackupFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "first"), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewApp(filepath.Join(root, "first", "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	oldBackups := filepath.Join(store.DataDirectory(), "backups")
	if err := os.MkdirAll(oldBackups, 0700); err != nil {
		t.Fatal(err)
	}
	name := "study-20261104-120000.db"
	if err := os.WriteFile(filepath.Join(oldBackups, name), []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "second")
	if err := (&APIService{app: store, locationFile: filepath.Join(root, "storage-location.txt")}).SetDataDirectory(target); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(target, "backups", name))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "backup" {
		t.Fatalf("migrated backup = %q", content)
	}
	if _, err := os.Stat(oldBackups); !os.IsNotExist(err) {
		t.Fatalf("old backup directory remains: %v", err)
	}
}
