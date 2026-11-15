package main

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func httpCleanupWorkers() int {
	buffer := make([]byte, 1024*1024)
	n := runtime.Stack(buffer, true)
	return strings.Count(string(buffer[:n]), "github.com/wailsapp/wails/v3/pkg/application.(*HTTPTransport).cleanupChunks(")
}

func awaitHTTPWorkers(t *testing.T, expected int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for httpCleanupWorkers() != expected {
		select {
		case <-deadline:
			t.Fatalf("HTTP cleanup workers = %d, want %d", httpCleanupWorkers(), expected)
		default:
			runtime.Gosched()
		}
	}
}

// TestDesktopTransportPublicStartStopJoinsCleanupWorker 验证公开 transport 的 Start/Stop 与清理 worker 协同。
func TestDesktopTransportPublicStartStopJoinsCleanupWorker(t *testing.T) {
	before := httpCleanupWorkers()
	transport := newDesktopTransport()
	defer transport.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 公开的 Start 方法在构造时就接收 processor。
	// 这个 worker 生命周期测试中不发起任何请求。
	if err := transport.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	awaitHTTPWorkers(t, before+1)
	cancel()
	// Wails 3.0.0 的 HTTPTransport 不使用 ctx 来停止 cleanupChunks。
	if httpCleanupWorkers() != before+1 {
		t.Fatal("transport behavior differs from reviewed Wails source")
	}
	var callers sync.WaitGroup
	for i := 0; i < 8; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := transport.Stop(); err != nil {
				t.Error(err)
			}
		}()
	}
	callers.Wait()
	if httpCleanupWorkers() != before {
		t.Fatal("Stop returned before the cleanup worker exited")
	}
	if err := transport.Stop(); err != nil {
		t.Fatal("repeated Stop", err)
	}
}
