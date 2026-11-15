package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestDesktopQuitDrainsBeforeSecondQuitAndKeepsMainThreadAvailable 验证第二次 Quit 前先排空任务，且主线程保持可用。
func TestDesktopQuitDrainsBeforeSecondQuitAndKeepsMainThreadAvailable(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, done, ok := store.tasks.enter()
	if !ok {
		t.Fatal("work rejected before shutdown")
	}
	var prepareCount, quitCount atomic.Int32
	quit := make(chan bool, 1)
	prepare := func() {
		prepareCount.Add(1)
		if err := store.database.WithRead(func(db *sql.DB, _ string) error { return db.Ping() }); err != nil {
			t.Error("database closed before workers drained", err)
		}
	}
	secondQuit := func() {
		quitCount.Add(1)
		quit <- store.tasks.shouldQuit(func() { t.Error("preparation repeated") }, func() { t.Error("quit repeated") })
	}
	if store.tasks.shouldQuit(prepare, secondQuit) {
		t.Fatal("first quit was allowed")
	}
	<-ctx.Done()
	// 这段代码代表主线程完成一个 InvokeSync 请求。
	// 由于 shouldQuit 返回时没有等待 done，因此它可以继续执行。
	for i := 0; i < 10; i++ {
		if store.tasks.shouldQuit(prepare, secondQuit) {
			t.Fatal("quit allowed before work completed")
		}
	}
	if _, _, ok := store.tasks.enter(); ok {
		t.Fatal("new work admitted during shutdown")
	}
	if prepareCount.Load() != 0 {
		t.Fatal("preparation ran before admitted work ended")
	}
	if store.tasks.readyToQuit() {
		t.Fatal("native window close allowed before preparation")
	}
	done()
	if !readAPIResult(t, quit) {
		t.Fatal("second quit was not allowed")
	}
	store.tasks.stopAndWait()
	if prepareCount.Load() != 1 || quitCount.Load() != 1 {
		t.Fatal("shutdown ran more than once")
	}
	if !store.tasks.readyToQuit() {
		t.Fatal("native window teardown was not allowed after preparation")
	}
	if !store.tasks.shouldQuit(prepare, secondQuit) {
		t.Fatal("repeated completed quit should pass")
	}
}

// TestDesktopShutdownCancelsDelayedWorkAndWaitsForActiveWork 验证关闭时取消延迟任务并等待进行中的任务。
func TestDesktopShutdownCancelsDelayedWorkAndWaitsForActiveWork(t *testing.T) {
	var tasks desktopTasks
	var ran atomic.Bool
	tasks.after(time.Hour, func() { ran.Store(true) })
	started, release := make(chan struct{}), make(chan struct{})
	tasks.start(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		<-release
	})
	<-started
	stopped := make(chan struct{})
	go func() { tasks.stopAndWait(); close(stopped) }()
	tasks.mu.Lock()
	ctx := tasks.ctx
	tasks.mu.Unlock()
	<-ctx.Done()
	select {
	case <-stopped:
		t.Fatal("shutdown ignored an active task")
	default:
	}
	close(release)
	readAPIResult(t, stopped)
	tasks.stopAndWait()
	if ran.Load() {
		t.Fatal("canceled delayed work ran")
	}
}

type blockedLogWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w blockedLogWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}

// TestDesktopShutdownWaitsForActualAPICall 验证关闭会等待正在进行的 API 调用。
func TestDesktopShutdownWaitsForActualAPICall(t *testing.T) {
	api := newAPITestService(t)
	writer := blockedLogWriter{make(chan struct{}), make(chan struct{})}
	previous := log.Writer()
	log.SetOutput(writer)
	defer log.SetOutput(previous)
	response := make(chan string, 1)
	go func() { response <- api.Call("logs", http.MethodPost, `{"message":"in flight"}`) }()
	<-writer.entered
	quit := make(chan struct{})
	if api.app.tasks.shouldQuit(func() {}, func() { close(quit) }) {
		t.Fatal("initial quit allowed")
	}
	select {
	case <-quit:
		t.Fatal("quit overtook API call")
	default:
	}
	close(writer.release)
	if got := readAPIResult(t, response); got != "{\"ok\":true}\n" {
		t.Fatal(got)
	}
	readAPIResult(t, quit)
	if got := api.Call("logs", http.MethodPost, "{}"); got != `{"error":"应用正在退出"}` {
		t.Fatal(got)
	}
}

// TestDesktopTaskAdmissionAndRepeatedQuitAreRaceSafe 验证任务接纳与重复 Quit 的并发安全。
func TestDesktopTaskAdmissionAndRepeatedQuitAreRaceSafe(t *testing.T) {
	for attempt := 0; attempt < 30; attempt++ {
		var tasks desktopTasks
		_, hold, _ := tasks.enter()
		barrier := make(chan struct{})
		var callers sync.WaitGroup
		var prepares, quits atomic.Int32
		finished := make(chan struct{})
		for i := 0; i < 12; i++ {
			callers.Add(1)
			go func() {
				defer callers.Done()
				<-barrier
				tasks.start(func(ctx context.Context) { <-ctx.Done() })
				tasks.shouldQuit(func() { prepares.Add(1) }, func() { quits.Add(1); close(finished) })
			}()
		}
		close(barrier)
		callers.Wait()
		hold()
		tasks.stopAndWait()
		readAPIResult(t, finished)
		tasks.mu.Lock()
		ready := tasks.ready
		tasks.mu.Unlock()
		if !ready || prepares.Load() != 1 || quits.Load() != 1 {
			t.Fatal("preparation was not exactly once")
		}
	}
}

type lifecycleInstance struct {
	listenErr error
	stops     int
	onListen  func(func())
}

func (g *lifecycleInstance) ListenForSecondLaunch(show func()) error {
	if g.onListen != nil {
		g.onListen(show)
	}
	return g.listenErr
}
func (g *lifecycleInstance) StopListening() error { g.stops++; return nil }
func (g *lifecycleInstance) Close() error         { return nil }

type observedHTTPTransport struct {
	*desktopTransport
	starts  int
	stopErr error
}

func (transport *observedHTTPTransport) Start(ctx context.Context, processor *application.MessageProcessor) error {
	transport.starts++
	return transport.desktopTransport.Start(ctx, processor)
}

func (transport *observedHTTPTransport) Stop() error {
	return errors.Join(transport.desktopTransport.Stop(), transport.stopErr)
}

// TestDesktopStartupFailureReleasesAdoptedDatabase 验证启动失败时释放已接管的数据库连接。
func TestDesktopStartupFailureReleasesAdoptedDatabase(t *testing.T) {
	if scenario := os.Getenv("STUDYFLOW_LIFECYCLE_SCENARIO"); scenario != "" {
		base := os.Getenv("STUDYFLOW_LIFECYCLE_BASE")
		paths := startupDatabasePaths{defaultDir: filepath.Join(base, "data"), locationFile: filepath.Join(base, "location"), installDir: filepath.Join(base, "install"), legacyConfigDir: filepath.Join(base, "legacy")}
		database, err := openStartupDatabase(paths, nil)
		if err != nil {
			t.Fatal(err)
		}
		store := newApp(database)
		var original *sql.DB
		if err := database.WithRead(func(db *sql.DB, _ string) error { original = db; return nil }); err != nil {
			t.Fatal(err)
		}
		defaultDir := filepath.Join(base, "ringtone-data")
		guard := &lifecycleInstance{}
		failure := errors.New("injected " + scenario)
		cleanupFailure := errors.New("injected transport cleanup failure")
		transport := &observedHTTPTransport{desktopTransport: newDesktopTransport()}
		switch scenario {
		case "data-directory":
			if err := os.WriteFile(defaultDir, []byte("occupied"), 0600); err != nil {
				t.Fatal(err)
			}
		case "ringtone":
			if err := os.MkdirAll(filepath.Join(defaultDir, "activity-ringtone.mp3"), 0700); err != nil {
				t.Fatal(err)
			}
		case "second-instance":
			guard.listenErr = failure
		case "tray-settings":
			if _, err := original.Exec("DROP TABLE settings"); err != nil {
				t.Fatal(err)
			}
		case "early-second-launch":
			guard.onListen = func(show func()) {
				if application.Get() != nil {
					t.Error("listener initialized after Wails construction")
				}
				show() // 此时尚无原生窗口，因此必须只入队一次唤醒。
			}
		case "cleanup-error":
			transport.stopErr = cleanupFailure
		case "prepare-and-cleanup-error":
			guard.listenErr = failure
			transport.stopErr = cleanupFailure
		}
		var ran bool
		workersBefore := httpCleanupWorkers()
		err = runDesktop(store, guard, base, defaultDir, paths.locationFile, transport, func(_ *application.App) error {
			ran = true
			if transport.starts != 1 {
				t.Error("Wails did not start the injected public transport")
			}
			awaitHTTPWorkers(t, workersBefore+1)
			if scenario == "run-error-with-task" {
				started := make(chan struct{})
				store.tasks.start(func(ctx context.Context) {
					if err := store.database.WithRead(func(db *sql.DB, _ string) error {
						close(started)
						<-ctx.Done()
						// 清理必须等待这个已接纳的数据库使用者。
						return db.Ping()
					}); err != nil {
						t.Error("Run failure closed a borrowed database", err)
					}
				})
				<-started
				return failure
			}
			if scenario == "run-error" || scenario == "cleanup-error" {
				return failure
			}
			if scenario == "normal-return" {
				// 同时触发 Wails 关闭与我们回退逻辑对同一个 Stop 的调用。
				return transport.Stop()
			}
			return nil
		})
		if scenario == "normal-return" || scenario == "early-second-launch" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("startup failure was swallowed")
		}
		if (scenario == "second-instance" || scenario == "run-error") && !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if (scenario == "cleanup-error" || scenario == "prepare-and-cleanup-error") && (!errors.Is(err, failure) || !errors.Is(err, cleanupFailure)) {
			t.Fatal("cleanup overwrote the run error", err)
		}
		if ran != (scenario == "run-error" || scenario == "run-error-with-task" || scenario == "normal-return" || scenario == "early-second-launch" || scenario == "cleanup-error") {
			t.Fatal("unexpected desktop run", ran)
		}
		if !ran && (application.Get() != nil || transport.starts != 0) {
			t.Fatal("local preparation failure reached application.New")
		}
		if httpCleanupWorkers() != workersBefore {
			t.Fatal("startup or Run failure leaked the real HTTPTransport worker")
		}
		if original.Ping() == nil || original.Stats().OpenConnections != 0 {
			t.Fatal("adopted connection was not closed")
		}
		if guard.stops != 1 {
			t.Fatal("listener stop count", guard.stops)
		}
		if err := store.Close(); err != nil {
			t.Fatal("repeated database close", err)
		}
		return
	}
	for _, scenario := range []string{"data-directory", "ringtone", "second-instance", "tray-settings", "run-error", "normal-return", "early-second-launch", "cleanup-error", "prepare-and-cleanup-error", "run-error-with-task"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestDesktopStartupFailureReleasesAdoptedDatabase$")
			cmd.Env = append(os.Environ(), "STUDYFLOW_LIFECYCLE_SCENARIO="+scenario, "STUDYFLOW_LIFECYCLE_BASE="+t.TempDir())
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}

// TestSecondLaunchQueuesUntilActivationAndStopsOnShutdown 验证二次启动在激活前只入队、关闭后不再唤醒。
func TestSecondLaunchQueuesUntilActivationAndStopsOnShutdown(t *testing.T) {
	var tasks desktopTasks
	launch := &secondLaunch{tasks: &tasks}
	var shown atomic.Int32
	launch.notify()
	launch.notify()
	if shown.Load() != 0 {
		t.Fatal("native wake ran before activation")
	}
	launch.activate(func() { shown.Add(1) })
	if shown.Load() != 1 {
		t.Fatal("pending launches were not coalesced")
	}
	launch.notify()
	if shown.Load() != 2 {
		t.Fatal("active listener did not wake window")
	}
	tasks.stopAndWait()
	launch.notify()
	launch.activate(func() { shown.Add(1) })
	if shown.Load() != 2 {
		t.Fatal("listener woke a window during shutdown")
	}
}

// TestSecondLaunchActiveCallbackDrainsBeforeQuit 验证激活回调先执行完毕、Quit 才继续。
func TestSecondLaunchActiveCallbackDrainsBeforeQuit(t *testing.T) {
	var tasks desktopTasks
	launch := &secondLaunch{tasks: &tasks}
	entered, release, callbackDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	launch.activate(func() { close(entered); <-release })
	go func() { launch.notify(); close(callbackDone) }()
	<-entered
	quit := make(chan struct{})
	if tasks.shouldQuit(func() {}, func() { close(quit) }) {
		t.Fatal("initial quit passed")
	}
	select {
	case <-quit:
		t.Fatal("quit overtook active wake callback")
	default:
	}
	close(release)
	readAPIResult(t, callbackDone)
	readAPIResult(t, quit)
	tasks.stopAndWait()
}

// 保留对测试 writer 的编译期检查，同时不替换全局钩子。
var _ io.Writer = blockedLogWriter{}
