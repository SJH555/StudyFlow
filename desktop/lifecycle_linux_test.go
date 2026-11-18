//go:build linux && !android

package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exam_notify/internal/infrastructure/system"
)

func TestRunStartupFailureReleasesLoggerAndSingleInstance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPIMAGE", filepath.Join(home, "install", "StudyFlow.AppImage"))
	base := filepath.Join(home, ".StudyFlow")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(home, "missing")
	location := filepath.Join(base, "storage-location.txt")
	if err := os.WriteFile(location, []byte(selected+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := log.Writer()
	err := runWithStartupPresenter(func(store *App, _ system.InstanceGuard, _, _, _ string, startup *startupUpgrade) error {
		defer store.Close()
		state := startup.snapshot()
		if state.Phase != "blocked" {
			t.Errorf("missing database did not block startup: %+v", state)
		}
		return errors.New(state.Detail)
	})
	if err == nil || !strings.Contains(err.Error(), "缺少 study.db") {
		t.Fatal(err)
	}
	if log.Writer() != previous || appErrorLog != nil {
		t.Fatal("startup retained the closed logger")
	}
	guard, primary, err := system.AcquireSingleInstance(base)
	if err != nil || !primary {
		t.Fatal("startup leaked single-instance claim", primary, err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(selected, "study.db")); !os.IsNotExist(err) {
		t.Fatal("startup created missing selected database", err)
	}
	if b, err := os.ReadFile(location); err != nil || string(b) != selected+"\n" {
		t.Fatal("startup changed saved location", err)
	}
}
