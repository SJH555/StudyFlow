//go:build windows

package system

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestWindowsInstanceStopJoinsBeforeClosingHandles(t *testing.T) {
	// 使用未命名的句柄，避免测试触碰到正在运行应用的命名单实例互斥体或唤醒事件。
	mutex, err := windows.CreateMutex(nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	wake, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		windows.CloseHandle(mutex)
		t.Fatal(err)
	}
	g := &windowsInstanceGuard{mutex: mutex, wake: wake, done: make(chan struct{})}
	if err := g.ListenForSecondLaunch(func() {}); err != nil {
		t.Fatal(err)
	}
	if err := g.StopListening(); err != nil {
		t.Fatal(err)
	}
	if _, err := windows.WaitForSingleObject(wake, 0); err != nil {
		t.Fatal("wake handle closed before final Close", err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal("repeated Close", err)
	}
}
