package system

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowStateCloseCancelsPendingCaptureAndRejectsNewSchedules(t *testing.T) {
	tracker := NewWindowStateTracker(filepath.Join(t.TempDir(), "window.json"), "linux", SavedWindowState{})
	var calls atomic.Int32
	tracker.schedule(func() { calls.Add(1) })
	tracker.Close(nil)
	tracker.schedule(func() { calls.Add(1) })
	tracker.Capture(nil) // Closed trackers must not touch native windows.
	tracker.Close(nil)
	if calls.Load() != 0 {
		t.Fatal("capture ran after close")
	}
}

func TestWindowStateCloseWaitsForRunningCapture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	tracker := NewWindowStateTracker(path, "linux", SavedWindowState{})
	started, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	tracker.schedule(func() {
		close(started)
		<-release
		if err := os.WriteFile(path, []byte("capture finished"), 0600); err != nil {
			t.Error(err)
		}
	})
	tracker.mu.Lock()
	tracker.timer.Reset(0)
	tracker.mu.Unlock()
	<-started
	go func() { tracker.Close(nil); close(closed) }()
	deadline := time.After(5 * time.Second)
	for {
		tracker.mu.Lock()
		stopping := tracker.closed
		tracker.mu.Unlock()
		if stopping {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Close did not begin")
		default:
			runtime.Gosched()
		}
	}
	select {
	case <-closed:
		t.Fatal("Close skipped the running capture")
	default:
	}
	close(release)
	<-closed
	if b, err := os.ReadFile(path); err != nil || string(b) != "capture finished" {
		t.Fatal("capture did not complete", err)
	}
	var callers sync.WaitGroup
	for i := 0; i < 5; i++ {
		callers.Add(1)
		go func() { defer callers.Done(); tracker.Close(nil) }()
	}
	callers.Wait()
}

func TestWindowStateRestoresSizeAndSupportedPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	want := SavedWindowState{Width: 1420, Height: 860, X: -1200, Y: 140, HasPosition: true}
	if err := writeWindowState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadWindowState(path)
	if err != nil || got != want {
		t.Fatalf("read state = %+v, %v", got, err)
	}
	options := application.WebviewWindowOptions{Width: 1380, Height: 920, MinWidth: 1000, MinHeight: 700}
	ApplyWindowState(&options, got, "windows")
	if options.Width != want.Width || options.Height != want.Height || options.X != want.X || options.Y != want.Y || options.InitialPosition != application.WindowXY {
		t.Fatalf("Windows window state was not restored: %+v", options)
	}
}

func TestWaylandRestoresSizeWithoutPosition(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("GDK_BACKEND", "wayland")
	options := application.WebviewWindowOptions{Width: 1380, Height: 920, MinWidth: 1000, MinHeight: 700}
	ApplyWindowState(&options, SavedWindowState{Width: 1200, Height: 760, X: 800, Y: 400, HasPosition: true}, "linux")
	if options.Width != 1200 || options.Height != 760 || options.InitialPosition == application.WindowXY {
		t.Fatalf("Wayland state applied incorrectly: %+v", options)
	}
}

func TestWindowStateRejectsInvalidSize(t *testing.T) {
	options := application.WebviewWindowOptions{Width: 1380, Height: 920, MinWidth: 1000, MinHeight: 700}
	ApplyWindowState(&options, SavedWindowState{Width: 100, Height: 0}, "windows")
	if options.Width != 1380 || options.Height != 920 {
		t.Fatalf("invalid size replaced defaults: %+v", options)
	}
}

func TestWindowStateRestoresMaximisedState(t *testing.T) {
	options := application.WebviewWindowOptions{Width: 1380, Height: 920, MinWidth: 1000, MinHeight: 700}
	ApplyWindowState(&options, SavedWindowState{Width: 1200, Height: 800, Maximised: true}, "windows")
	if options.StartState != application.WindowStateMaximised || options.Width != 1200 || options.Height != 800 {
		t.Fatalf("maximised window was not restored: %+v", options)
	}
}
