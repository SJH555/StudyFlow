package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoggerCloseStopsRotationAndIsIdempotent(t *testing.T) {
	l, err := StartErrorLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := l.file
	var callers sync.WaitGroup
	for i := 0; i < 8; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := l.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	callers.Wait()
	select {
	case <-l.done:
	default:
		t.Fatal("rotation worker outlived Close")
	}
	if _, err := file.WriteString("closed"); !errors.Is(err, os.ErrClosed) {
		t.Fatal("log file is still open", err)
	}
	if _, err := l.Write([]byte("late write")); !errors.Is(err, os.ErrClosed) {
		t.Fatal("closed logger accepted write", err)
	}
	l.day = "2000-01-01"
	if err := l.rotate(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("closed logger rotated", err)
	}
}

func TestLoggerCloseRetainsFirstCloseError(t *testing.T) {
	l, err := StartErrorLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.file.Close(); err != nil {
		t.Fatal(err)
	}
	first := l.Close()
	if !errors.Is(first, os.ErrClosed) {
		t.Fatal("missing close error", first)
	}
	if next := l.Close(); next != first {
		t.Fatal("first close error changed", next)
	}
}

func TestLoggerStartupFailureDoesNotCreateLogResources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(path, []byte("user file"), 0600); err != nil {
		t.Fatal(err)
	}
	if l, err := StartErrorLog(path); err == nil || l != nil {
		t.Fatal("invalid log directory accepted")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "user file" {
		t.Fatal("startup changed user file", err)
	}
}
