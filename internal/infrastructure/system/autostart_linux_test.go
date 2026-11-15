//go:build linux

package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutostartOnlyRemovesItsOwnEntry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := autostartPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("someone else's startup entry"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetAutostart(true); err == nil {
		t.Fatal("overwrote an unrelated entry")
	}
	if err := SetAutostart(false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "someone else's startup entry" {
		t.Fatalf("unrelated entry changed: %q, %v", data, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	enabled, err := AutostartEnabled()
	if err != nil || !enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), autostartMarker) {
		t.Fatalf("missing marker: %v", err)
	}
	if err := SetAutostart(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("managed entry remains: %v", err)
	}
}

func TestAutostartRefusesSymlink(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := autostartPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "other.desktop")
	if err := os.WriteFile(target, []byte(autostartMarker), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := SetAutostart(false); err == nil {
		t.Fatal("removed through a symlink")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(target); err != nil {
		t.Fatal(err)
	}
}
