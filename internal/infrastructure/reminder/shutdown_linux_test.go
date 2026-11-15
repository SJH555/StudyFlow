//go:build linux

package reminder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReminderShutdownReapsPlaybackAndNotificationWorkers(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ffplay"), []byte("#!/bin/sh\nexec /bin/sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "notify-send"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer Shutdown()
	Start(ActivitySound)
	player.mu.Lock()
	cmd := player.cmd
	player.mu.Unlock()
	if cmd == nil {
		t.Fatal("fake playback process did not start")
	}
	Notify("test", "notification")
	Shutdown()
	if cmd.ProcessState == nil {
		t.Fatal("Shutdown returned before cmd.Wait completed")
	}
	player.mu.Lock()
	closed, playing := player.closed, player.cmd != nil
	player.mu.Unlock()
	if !closed || playing {
		t.Fatal("audio still accepts work or retains playback")
	}
	Start(PlanSound)
	Notify("late", "must not start")
	Shutdown()
	player.mu.Lock()
	defer player.mu.Unlock()
	if player.cmd != nil {
		t.Fatal("playback restarted after Shutdown")
	}
}
