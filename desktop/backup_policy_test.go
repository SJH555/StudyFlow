package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupChecksSerializeWithDatabaseRequests(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := testExec(store, "INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1')"); err != nil {
		t.Fatal(err)
	}
	api := &APIService{app: store}
	now := time.Now().Truncate(time.Second)
	start := make(chan struct{})
	done := make(chan error, 3)
	for range 2 {
		go func() {
			<-start
			done <- store.scheduler.BackupCheck(now)
		}()
	}
	go func() {
		<-start
		response := api.Call("settings", "PUT", `{"activityInterval":"45"}`)
		if strings.TrimSpace(response) != `{"ok":true}` {
			done <- fmt.Errorf("settings response: %s", response)
			return
		}
		done <- nil
	}()
	close(start)

	timeout := time.After(5 * time.Second)
	for range 3 {
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-timeout:
			t.Error("concurrent backup or database request did not complete")
			return
		}
	}
	dir := filepath.Join(store.DataDirectory(), "backups")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("concurrent checks produced %d backups, err=%v", len(entries), err)
	}
}
