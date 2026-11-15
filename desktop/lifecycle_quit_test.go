package main

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type quitOrderTransport struct {
	*desktopTransport
	tasks *desktopTasks
	stops atomic.Int32
	t     *testing.T
}

func (transport *quitOrderTransport) Stop() error {
	// 这是 runDesktop 的回退 Stop，而非 Wails 的原生关闭钩子。
	select {
	case <-transport.tasks.shutdownDone:
	default:
		transport.t.Error("fallback closed transport before the coordinator finished")
	}
	transport.stops.Add(1)
	return transport.desktopTransport.Stop()
}

func awaitDesktopRunReturn(t *testing.T, tasks *desktopTasks) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		tasks.mu.Lock()
		returned := tasks.runReturned
		tasks.mu.Unlock()
		if returned {
			return
		}
		select {
		case <-deadline:
			t.Fatal("Run fallback did not enter stopAndWait")
		default:
			runtime.Gosched()
		}
	}
}

// TestRunReturnAndSecondQuitOrdering 验证 Run 返回与第二次 Quit 之间的顺序关系。
func TestRunReturnAndSecondQuitOrdering(t *testing.T) {
	if scenario := os.Getenv("STUDYFLOW_QUIT_ORDER_TEST"); scenario != "" {
		testRunReturnAndSecondQuitOrdering(t, scenario)
		return
	}
	// 每个子进程获得独立的 Wails 单实例。真实的原生事件循环
	// 仅被一个由测试栅栏控制的、每次调用独立的队列所替代。
	for _, scenario := range []string{"before-dispatch", "queued", "executing", "normal"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunReturnAndSecondQuitOrdering$")
			cmd.Env = append(os.Environ(), "STUDYFLOW_QUIT_ORDER_TEST="+scenario)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}

func testRunReturnAndSecondQuitOrdering(t *testing.T, scenario string) {
	base := t.TempDir()
	store, err := NewApp(filepath.Join(base, "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	var raw *sql.DB
	if err := store.database.WithRead(func(db *sql.DB, _ string) error { raw = db; return nil }); err != nil {
		t.Fatal(err)
	}
	transport := &quitOrderTransport{desktopTransport: newDesktopTransport(), tasks: &store.tasks, t: t}
	guard := &lifecycleInstance{}
	runFailure := errors.New("injected concurrent Run failure")
	prepared := make(chan struct{})
	approachingQuit := make(chan struct{})
	allowDispatch := make(chan struct{})
	queued := make(chan func(), 1)
	enqueued := make(chan struct{})
	quitEntered := make(chan struct{})
	allowQuitReturn := make(chan struct{})
	nativeReturned := make(chan struct{})
	scopeReturned := make(chan error, 1)
	var prepares, dispatches, quits atomic.Int32
	quit := func() {
		quits.Add(1)
		if !store.tasks.shouldQuit(func() { t.Error("preparation repeated") }, func() { t.Error("coordinator repeated") }) {
			t.Error("second ShouldQuit was not allowed")
		}
		close(quitEntered)
		if scenario == "executing" {
			<-allowQuitReturn
		}
		if err := raw.Ping(); err != nil {
			t.Error("fallback closed database while second Quit was still running", err)
		}
		close(nativeReturned)
	}
	dispatch := func(callback func()) {
		dispatches.Add(1)
		queued <- callback
		close(enqueued)
	}
	go func() {
		scopeReturned <- runDesktop(store, guard, base, filepath.Join(base, "sounds"), filepath.Join(base, "location"), transport, func(_ *application.App) error {
			if store.tasks.shouldQuit(func() {
				prepares.Add(1)
				close(prepared)
			}, func() {
				close(approachingQuit)
				if scenario == "before-dispatch" {
					<-allowDispatch
				}
				store.tasks.quitOnMainThread(dispatch, quit)
			}) {
				t.Error("first ShouldQuit was allowed")
			}
			<-prepared
			<-approachingQuit
			switch scenario {
			case "queued":
				<-enqueued
			case "executing":
				<-quitEntered
			case "normal":
				<-nativeReturned
				return nil
			}
			return runFailure
		})
	}()
	<-approachingQuit
	if scenario == "executing" || scenario == "normal" {
		callback := readAPIResult(t, queued)
		go callback()
		<-quitEntered
	}
	if scenario == "before-dispatch" || scenario == "executing" {
		awaitDesktopRunReturn(t, &store.tasks)
		// 准备工作已完成，但受控的 Quit 回调仍被阻塞时，
		// 协调协程与回退逻辑都不得宣称完成。
		select {
		case <-store.tasks.drained:
		default:
			t.Fatal("preparation was not complete")
		}
		select {
		case <-store.tasks.shutdownDone:
			t.Fatal("coordinator completed before its Quit callback returned")
		default:
		}
		if transport.stops.Load() != 0 || raw.Ping() != nil {
			t.Fatal("resources closed while the exit coordinator was blocked")
		}
		if scenario == "before-dispatch" {
			close(allowDispatch)
		} else {
			close(allowQuitReturn)
		}
	}
	err = readAPIResult(t, scopeReturned)
	if scenario == "normal" {
		if err != nil {
			t.Fatal(err)
		}
	} else if !errors.Is(err, runFailure) {
		t.Fatal("Run error was lost", err)
	}
	if prepares.Load() != 1 || transport.stops.Load() != 1 || raw.Ping() == nil {
		t.Fatal("incorrect preparation or final cleanup counts")
	}
	if scenario == "queued" {
		// 即便回调留在已停止的原生队列中，稍后（回退逻辑已关闭所有资源后）
		// 再被分发也必须是无害的。
		callback := readAPIResult(t, queued)
		callback()
	}
	if scenario == "before-dispatch" && dispatches.Load() != 0 {
		t.Fatal("coordinator dispatched after Run had returned")
	}
	if (scenario == "queued" || scenario == "before-dispatch") && quits.Load() != 0 {
		t.Fatal("a canceled main-thread Quit was executed")
	}
	if (scenario == "executing" || scenario == "normal") && quits.Load() != 1 {
		t.Fatal("second Quit did not execute exactly once")
	}
}
