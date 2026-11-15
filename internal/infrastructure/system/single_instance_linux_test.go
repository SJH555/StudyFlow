//go:build linux

package system

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSingleInstanceStopWaitsForCallbackAndRetainsClaim(t *testing.T) {
	base := t.TempDir()
	guard, primary, err := AcquireSingleInstance(base)
	if err != nil || !primary {
		t.Fatal(primary, err)
	}
	defer guard.Close()
	started, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	if err := guard.ListenForSecondLaunch(func() { close(started); <-release }); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", filepath.Join(base, "StudyFlow.sock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("show\n")); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	<-started
	go func() {
		if err := guard.StopListening(); err != nil {
			t.Error(err)
		}
		close(stopped)
	}()
	// 已在执行的回调在返回前拥有其资源。
	select {
	case <-stopped:
		t.Fatal("listener abandoned callback")
	default:
	}
	close(release)
	<-stopped
	lock, err := os.OpenFile(filepath.Join(base, "StudyFlow.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		t.Fatal("StopListening released the instance claim")
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal("repeated close", err)
	}
}

func TestSingleInstanceClaimsAndReopens(t *testing.T) {
	base := t.TempDir()
	first, primary, err := AcquireSingleInstance(base)
	if err != nil || !primary {
		t.Fatalf("first claim: primary=%v err=%v", primary, err)
	}
	defer first.Close()
	opened := make(chan struct{}, 1)
	if err := first.ListenForSecondLaunch(func() { opened <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	second, primary, err := AcquireSingleInstance(base)
	if err != nil || primary || second != nil {
		t.Fatalf("second claim: guard=%v primary=%v err=%v", second, primary, err)
	}
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("second launch did not notify first")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, primary, err := AcquireSingleInstance(base)
	if err != nil || !primary {
		t.Fatalf("reclaim after close: primary=%v err=%v", primary, err)
	}
	defer third.Close()
	if _, err := os.Stat(filepath.Join(base, "StudyFlow.lock")); err != nil {
		t.Fatal(err)
	}
}
