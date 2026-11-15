package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnableAutomaticBackupRespondsWithoutDeadlock(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	response := make(chan string, 1)
	go func() {
		response <- (&APIService{app: store}).Call("settings", "PUT", `{"autoBackupEnabled":"1"}`)
	}()
	select {
	case result := <-response:
		if strings.TrimSpace(result) != `{"ok":true}` {
			t.Fatalf("settings response: %s", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("enabling backup blocked the API request")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(filepath.Join(store.DataDirectory(), "backups"))
		if err == nil && len(entries) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("initial backup was not created")
}
