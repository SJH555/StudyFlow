package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/diagnostics"
	"exam_notify/internal/infrastructure/persistence"
)

const apiTestTimeout = 5 * time.Second

func newAPITestService(t *testing.T) *APIService {
	t.Helper()
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return &APIService{app: store}
}

func useAPITestLog(t *testing.T) *diagnostics.Logger {
	t.Helper()
	logger, err := diagnostics.StartErrorLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	previousLogger, previousOutput := appErrorLog, log.Writer()
	appErrorLog = logger
	log.SetOutput(logger)
	t.Cleanup(func() {
		appErrorLog = previousLogger
		log.SetOutput(previousOutput)
		if err := errors.Join(debug.SetCrashOutput(nil, debug.CrashOptions{}), logger.Close()); err != nil {
			t.Error(err)
		}
	})
	return logger
}

func readAPIResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(apiTestTimeout):
		t.Fatal("timed out waiting for API test barrier")
		var zero T
		return zero
	}
}

// TestAPILogsKeepBindingContract 验证日志相关 API 保持对前端的绑定契约。
func TestAPILogsKeepBindingContract(t *testing.T) {
	api := newAPITestService(t)
	logger := useAPITestLog(t)
	message := strings.Repeat("a", 2000)
	body, err := json.Marshal(map[string]string{"message": "  " + message + "cut-off  "})
	if err != nil {
		t.Fatal(err)
	}
	if got := api.Call("/logs/", http.MethodPost, string(body)); got != "{\"ok\":true}\n" {
		t.Fatalf("POST logs = %q", got)
	}
	var list struct {
		Directory     string             `json:"directory"`
		RetentionDays int                `json:"retentionDays"`
		Files         []diagnostics.File `json:"files"`
	}
	if raw := api.Call("logs", http.MethodGet, ""); json.Unmarshal([]byte(raw), &list) != nil ||
		list.Directory != logger.Directory() || list.RetentionDays != 7 || len(list.Files) != 1 ||
		list.Files[0].Size <= 0 || list.Files[0].Modified == "" {
		t.Fatalf("GET logs = %q", raw)
	}
	var detail struct {
		Content string `json:"content"`
	}
	if raw := api.Call("//logs///?name="+list.Files[0].Name, http.MethodGet, ""); json.Unmarshal([]byte(raw), &detail) != nil ||
		!strings.Contains(detail.Content, "UI error: "+message+"\n") || strings.Contains(detail.Content, "cut-off") {
		t.Fatalf("GET log detail = %q", raw)
	}
	// 通过 http.NewRequest，未指定方法时仍默认为 GET。
	if raw := api.Call("logs", "", ""); raw != api.Call("logs", http.MethodGet, "") {
		t.Fatalf("default method = %q", raw)
	}
	for _, tc := range []struct{ path, method, body, want string }{
		{"logs", http.MethodPost, "{", `{"error":"unexpected EOF"}`},
		{"logs?name=..%2Ferror-2026-11-08.log", http.MethodGet, "", `{"error":"invalid log file"}`},
		{"logs", http.MethodDelete, "", `{"error":"404 page not found"}`},
		{"logs", "get", "", `{"error":"404 page not found"}`},
		{"api/logs", http.MethodGet, "", `{"error":"404 page not found"}`},
		{"logs/extra", http.MethodPost, "{}", `{"error":"404 page not found"}`},
		{"reminder/ack", http.MethodGet, "", `{"error":"404 page not found"}`},
		{"reminder//ack", http.MethodPost, "{}", `{"error":"404 page not found"}`},
	} {
		if got := api.Call(tc.path, tc.method, tc.body); got != tc.want {
			t.Errorf("%s %s = %q, want %s", tc.method, tc.path, got, tc.want)
		}
	}
	appErrorLog = nil
	if got := api.Call("logs", http.MethodGet, ""); got != `{"error":"error log unavailable"}` {
		t.Fatalf("GET logs with no logger = %q", got)
	}
}

// TestDatabaseFreeAPIsCompleteWhileWriterHoldsLock 验证写锁被占用期间免数据库的 API 仍可完成。
func TestDatabaseFreeAPIsCompleteWhileWriterHoldsLock(t *testing.T) {
	api := newAPITestService(t)
	useAPITestLog(t)
	api.afterReminderAck = func(time.Duration, func()) {}

	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	writer := make(chan error, 1)
	go func() {
		writer <- api.app.database.WithWrite(func(db *sql.DB, _ string) error {
			close(entered)
			<-release
			_, err := db.Exec(`INSERT INTO subjects(name, created_at) VALUES('writer', '2026-11-08')`)
			return err
		})
	}()
	readAPIResult(t, entered)

	ordinary := make(chan string, 1)
	started := make(chan string, 1)
	go func() {
		var stack [128]byte
		n := runtime.Stack(stack[:], false)
		id, _, _ := strings.Cut(string(stack[:n]), " [")
		started <- id
		ordinary <- api.Call("snapshot", http.MethodGet, "")
	}()
	waitForAPIReadLock(t, readAPIResult(t, started), ordinary)
	assertDatabaseFreeAPIResults(t, api)

	unblock()
	if err := readAPIResult(t, writer); err != nil {
		t.Fatal(err)
	}
	if got := readAPIResult(t, ordinary); !strings.Contains(got, `"subjectsCount":1`) {
		t.Fatalf("ordinary route did not read after the writer: %q", got)
	}
}

// 协程启动信号并不能证明它真的发起了加锁。在走快速路径之前，
// 需确认普通请求已经阻塞在 Database.WithRead 上。
func waitForAPIReadLock(t *testing.T, id string, result <-chan string) {
	t.Helper()
	timeout := time.After(apiTestTimeout)
	stack := make([]byte, 64*1024)
	for {
		select {
		case got := <-result:
			t.Fatalf("ordinary request returned under the write lock: %q", got)
		case <-timeout:
			t.Fatal("ordinary request did not wait for the lifecycle read lock")
		default:
		}
		n := runtime.Stack(stack, true)
		if n == len(stack) {
			stack = make([]byte, 2*len(stack))
			continue
		}
		for _, entry := range strings.Split(string(stack[:n]), "\n\n") {
			if strings.HasPrefix(entry, id+" [sync.") && strings.Contains(entry, "sync.(*RWMutex).RLock(") &&
				strings.Contains(entry, "persistence.(*Database).WithRead(") {
				return
			}
		}
		runtime.Gosched()
	}
}

func assertDatabaseFreeAPIResults(t *testing.T, api *APIService) {
	t.Helper()
	for _, tc := range []struct{ path, method, body, want string }{
		{"logs", http.MethodGet, "", `"retentionDays":7`},
		{"logs", http.MethodPost, `{"message":"while database is unavailable"}`, "{\"ok\":true}\n"},
		{"/reminder/ack/", http.MethodPost, "{}", "{\"ok\":true}\n"},
	} {
		done := make(chan string, 1)
		go func() { done <- api.Call(tc.path, tc.method, tc.body) }()
		if got := readAPIResult(t, done); !strings.Contains(got, tc.want) {
			t.Errorf("%s %s = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

// TestDatabaseFreeAPIsCompleteAfterCloseOrPause 验证数据库关闭或暂停后免数据库的 API 仍可完成。
func TestDatabaseFreeAPIsCompleteAfterCloseOrPause(t *testing.T) {
	useAPITestLog(t)
	for _, state := range []struct {
		name  string
		cause error
	}{
		{"closed", persistence.ErrDatabaseClosed},
		{"paused", persistence.ErrDatabasePaused},
	} {
		t.Run(state.name, func(t *testing.T) {
			api := newAPITestService(t)
			api.afterReminderAck = func(time.Duration, func()) {}
			if state.cause == persistence.ErrDatabaseClosed {
				if err := api.app.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				// 持久化测试覆盖的是真实的不确定发布场景。这里在真实锁下写入结果状态，
				// 以便在不新增生产故障注入 API 的前提下测试绑定层。
				if err := api.app.database.WithWrite(func(_ *sql.DB, _ string) error {
					field := reflect.ValueOf(api.app.database).Elem().FieldByName("pauseErr")
					*(*error)(field.Addr().UnsafePointer()) = state.cause
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			assertDatabaseFreeAPIResults(t, api)
			want, err := json.Marshal(map[string]string{"error": state.cause.Error()})
			if err != nil {
				t.Fatal(err)
			}
			for _, req := range []struct{ path, method string }{
				{"snapshot", http.MethodGet},
				{"logs", http.MethodDelete},
				{"reminder/ack", http.MethodGet},
			} {
				if got := api.Call(req.path, req.method, ""); got != string(want) {
					t.Errorf("%s %s = %q, want %s", req.method, req.path, got, want)
				}
			}
		})
	}
}

// TestReminderAcknowledgementWritesResponseBeforeScheduling 验证提醒确认先写响应、后调度延迟任务。
func TestReminderAcknowledgementWritesResponseBeforeScheduling(t *testing.T) {
	api := newAPITestService(t)
	queue := &api.app.reminders
	noReview := func() (*dailyReview, error) { return nil, nil }
	for _, kind := range []string{"active", "waiting"} {
		if _, err := queue.Enqueue(deferredReminder{Kind: kind}, func() (*dailyReview, error) {
			return &dailyReview{}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if dispatch, err := queue.Release(noReview); err != nil || dispatch.Reminder == nil || dispatch.Reminder.Kind != "active" {
		t.Fatalf("seed active reminder = %+v, %v", dispatch, err)
	}
	recorder := httptest.NewRecorder()
	var order []string
	api.app.acknowledgeReminder(recorder, func() {
		order = append(order, "stop")
		if dispatch, err := queue.Release(noReview); err != nil || dispatch.Reminder != nil {
			t.Fatalf("queue was acknowledged before stop: %+v, %v", dispatch, err)
		}
	}, func() {
		order = append(order, "clear top")
		if dispatch, err := queue.Release(noReview); err != nil || dispatch.Reminder == nil || dispatch.Reminder.Kind != "waiting" {
			t.Fatalf("queue was not acknowledged before clearing top: %+v, %v", dispatch, err)
		}
		if recorder.Body.Len() != 0 {
			t.Fatal("response was written before clearing top")
		}
	}, func(delay time.Duration, release func()) {
		order = append(order, "schedule")
		if delay != 150*time.Millisecond || release == nil || recorder.Code != http.StatusOK ||
			recorder.Body.String() != "{\"ok\":true}\n" {
			t.Fatalf("schedule after %v with response %d %q", delay, recorder.Code, recorder.Body)
		}
	})
	if got := strings.Join(order, ", "); got != "stop, clear top, schedule" {
		t.Fatalf("acknowledgement order = %s", got)
	}
}

// TestReminderAcknowledgementDelayedQueryUsesSwitchedDatabase 验证切换目录后延迟的复盘查询使用新连接。
func TestReminderAcknowledgementDelayedQueryUsesSwitchedDatabase(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real directory switching is supported on desktop Linux")
	}
	api := newAPITestService(t)
	logger := useAPITestLog(t)
	scheduled := make(chan func(), 1)
	api.afterReminderAck = func(delay time.Duration, release func()) {
		if delay != 150*time.Millisecond {
			t.Errorf("acknowledgement delay = %v", delay)
		}
		scheduled <- release
	}
	if _, err := api.app.scheduler.PendingReview(time.Now()); err != nil {
		t.Fatalf("old database review query failed: %v", err)
	}
	if got := api.Call("reminder/ack", http.MethodPost, "{}"); got != "{\"ok\":true}\n" {
		t.Fatalf("acknowledgement response = %q", got)
	}
	release := readAPIResult(t, scheduled)

	root := t.TempDir()
	target := filepath.Join(root, "new")
	result, err := api.app.database.SwitchDirectory(context.Background(), target,
		filepath.Join(root, "storage-location.txt"), filepath.Join(root, "install"))
	if err != nil || !result.Committed || result.Warning != nil {
		t.Fatalf("real directory switch = %+v, %v", result, err)
	}
	// TEMP 视图提供一个仅存在于目标连接的查询标记，既不改动持久化 schema，
	// 也不依赖复盘在 06:00 的截止时间。
	if err := api.app.database.WithWrite(func(db *sql.DB, path string) error {
		if path != filepath.Join(target, "study.db") {
			return errors.New("incorrect active database path after switch")
		}
		_, err := db.Exec(`CREATE TEMP VIEW settings AS
			SELECT key, b2_target_connection_marker AS value FROM main.settings`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	release()
	files, err := logger.List()
	if err != nil || len(files) != 1 {
		t.Fatalf("delayed review did not query the marked target connection: %v, %v", files, err)
	}
	contents, err := logger.Read(files[0].Name)
	if err != nil || !strings.Contains(contents, "检查昨日总结失败") || !strings.Contains(contents, "b2_target_connection_marker") {
		t.Fatalf("delayed review did not read from the new connection: %q, %v", contents, err)
	}
}
