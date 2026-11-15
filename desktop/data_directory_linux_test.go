//go:build linux && !android

package main

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/storage"
)

func dataDirectoryBindingFixture(t *testing.T) (*APIService, string, string) {
	t.Helper()
	api := newAPITestService(t)
	root := t.TempDir()
	api.locationFile = filepath.Join(root, "storage-location.txt")
	install := filepath.Join(root, "install")
	t.Setenv("APPIMAGE", filepath.Join(install, "StudyFlow.AppImage"))
	return api, filepath.Join(root, "target"), install
}

// TestLinuxSetDataDirectoryRejectsEmptyInputBeforeReadingPending 验证读取 pending 记录前先拒绝空目录输入。
func TestLinuxSetDataDirectoryRejectsEmptyInputBeforeReadingPending(t *testing.T) {
	api, target, _ := dataDirectoryBindingFixture(t)
	oldDir := api.GetDataDirectory()
	writeStartupFixture(t, api.locationFile, oldDir+"\n")
	marker := api.locationFile + ".pending"
	writeStartupFixture(t, marker, "malformed pending record")

	for _, input := range []string{"", " \t\n\u3000"} {
		if err := api.SetDataDirectory(input); err == nil || err.Error() != "请选择一个有效的数据目录" {
			t.Errorf("input %q returned %v", input, err)
		}
	}
	if got := api.GetDataDirectory(); got != oldDir {
		t.Fatalf("active directory = %q, want %q", got, oldDir)
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(oldDir+"\n"))
	assertStartupFileUnchanged(t, marker, []byte("malformed pending record"))
	assertStartupPathMissing(t, target)
}

// TestLinuxSetDataDirectoryTrimsOuterWhitespace 验证切换目录时会去除输入首尾空白。
func TestLinuxSetDataDirectoryTrimsOuterWhitespace(t *testing.T) {
	api, target, _ := dataDirectoryBindingFixture(t)
	target += " with spaces"
	if err := api.SetDataDirectory(" \t" + target + " \u3000"); err != nil {
		t.Fatal(err)
	}
	if got := api.GetDataDirectory(); got != target {
		t.Fatalf("active directory = %q, want %q", got, target)
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(target+"\n"))
	assertStartupPathMissing(t, target+" \u3000")
}

// TestLinuxSetDataDirectoryCommitsAndCleansOnlyMigratedResources 验证切换后只清理已迁移的旧文件。
func TestLinuxSetDataDirectoryCommitsAndCleansOnlyMigratedResources(t *testing.T) {
	api, target, _ := dataDirectoryBindingFixture(t)
	oldDir := api.GetDataDirectory()
	oldDB := filepath.Join(oldDir, "study.db")
	backup := filepath.Join(oldDir, "backups", "study-20261108-120000.db")
	userFile := filepath.Join(oldDir, "backups", "notes.txt")
	writeStartupFixture(t, api.locationFile, oldDir+"\n")
	writeStartupFixture(t, backup, "old backup")
	writeStartupFixture(t, userFile, "keep user file")
	if _, err := testExec(api.app, `INSERT INTO subjects(name,created_at) VALUES('before switch','2026-11-08')`); err != nil {
		t.Fatal(err)
	}

	if err := api.SetDataDirectory(target); err != nil {
		t.Fatal(err)
	}
	if got := api.GetDataDirectory(); got != target {
		t.Fatalf("active directory = %q, want %q", got, target)
	}
	assertStartupFileUnchanged(t, api.locationFile, []byte(target+"\n"))
	assertStartupPathMissing(t, api.locationFile+".pending")
	assertStartupPathMissing(t, oldDB)
	assertStartupPathMissing(t, backup)
	assertStartupFileUnchanged(t, userFile, []byte("keep user file"))
	assertStartupFileUnchanged(t, filepath.Join(target, "backups", filepath.Base(backup)), []byte("old backup"))
	assertStartupPathMissing(t, filepath.Join(target, "backups", "notes.txt"))
	if got := api.Call("subjects", http.MethodGet, ""); !strings.Contains(got, "before switch") {
		t.Fatalf("subsequent API request did not read the new database: %s", got)
	}
	if err := api.SetDataDirectory(target); err != nil {
		t.Fatalf("same directory should be a successful no-op: %v", err)
	}
}

// TestLinuxSetDataDirectoryFailuresPreserveActiveDatabase 验证切换失败时保留当前活动数据库。
func TestLinuxSetDataDirectoryFailuresPreserveActiveDatabase(t *testing.T) {
	for _, scenario := range []string{"existing target", "install marker", "prepare backup collision"} {
		t.Run(scenario, func(t *testing.T) {
			api, target, install := dataDirectoryBindingFixture(t)
			oldDir := api.GetDataDirectory()
			oldPath := filepath.Join(oldDir, "study.db")
			writeStartupFixture(t, api.locationFile, oldDir+"\n")
			before := readStartupFixture(t, oldPath)
			var wantError, protected string
			switch scenario {
			case "existing target":
				protected = filepath.Join(target, "study.db")
				wantError = "目标目录中已存在 study.db"
			case "install marker":
				target = install
				wantError = "不能将安装目录用作新的数据目录"
			case "prepare backup collision":
				writeStartupFixture(t, filepath.Join(oldDir, "backups", "existing.db"), "source backup")
				protected = filepath.Join(target, "backups", "existing.db")
				wantError = "prepare database directory"
			}
			if protected != "" {
				writeStartupFixture(t, protected, "external file")
			}
			if err := api.SetDataDirectory(target); err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("switch failure = %v, want %q", err, wantError)
			}
			if got := api.GetDataDirectory(); got != oldDir {
				t.Fatalf("failed switch changed directory to %q", got)
			}
			if got := api.Call("snapshot", http.MethodGet, ""); !strings.Contains(got, `"subjectsCount":0`) {
				t.Fatalf("original database is unavailable: %s", got)
			}
			assertStartupFileUnchanged(t, oldPath, before)
			assertStartupFileUnchanged(t, api.locationFile, []byte(oldDir+"\n"))
			assertStartupPathMissing(t, api.locationFile+".pending")
			if protected != "" {
				assertStartupFileUnchanged(t, protected, []byte("external file"))
			}
			if scenario != "existing target" {
				assertStartupPathMissing(t, filepath.Join(target, "study.db"))
			}
		})
	}
}

// TestLinuxSetDataDirectoryRejectsExistingPendingWithoutTouchingIt 验证已存在 pending 记录时拒绝且不修改它。
func TestLinuxSetDataDirectoryRejectsExistingPendingWithoutTouchingIt(t *testing.T) {
	api, target, _ := dataDirectoryBindingFixture(t)
	old := api.GetDataDirectory()
	writeStartupFixture(t, api.locationFile, old+"\n")
	// 即使旧位置仍处于活动状态，Stage 留下的真实标记也可能存在。
	// 第二次绑定调用不得做任何准备或发布操作。
	candidate := filepath.Join(filepath.Dir(target), "previous-target", "study.db")
	seedStartupSQLite(t, candidate, true)
	stageBindingLocation(t, api, filepath.Join(old, "study.db"), candidate)
	marker := api.locationFile + ".pending"
	before := readStartupFixture(t, marker)
	if err := api.SetDataDirectory(target); !errors.Is(err, persistence.ErrDirectorySwitchPending) ||
		!strings.Contains(err.Error(), "待处理") {
		t.Fatalf("pending switch response = %v", err)
	}
	assertStartupFileUnchanged(t, marker, before)
	assertStartupFileUnchanged(t, api.locationFile, []byte(old+"\n"))
	assertStartupPathMissing(t, target)
}

func stageBindingLocation(t *testing.T, api *APIService, old, candidate string) {
	t.Helper()
	location, err := storage.OpenLocationSwitch(api.locationFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := location.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := location.Stage(old, candidate, filepath.Dir(os.Getenv("APPIMAGE"))); err != nil {
		t.Fatal(err)
	}
}

// TestLinuxDirectorySwitchResponseKeepsPublicationOutcomesDistinct 验证目录切换响应能区分不同的发布结果。
func TestLinuxDirectorySwitchResponseKeepsPublicationOutcomesDistinct(t *testing.T) {
	publishErr := errors.New("rename failed before commit")
	if err := directorySwitchResponse("target", persistence.DirectorySwitchResult{}, publishErr); err != publishErr {
		t.Fatalf("uncommitted publication error = %v", err)
	}

	cause := errors.New("publication could not be read back")
	uncertain := errors.Join(persistence.ErrDatabasePaused, cause)
	err := directorySwitchResponse("target", persistence.DirectorySwitchResult{}, uncertain)
	if !errors.Is(err, persistence.ErrDatabasePaused) || !errors.Is(err, cause) ||
		!strings.Contains(err.Error(), "无法确认") || !strings.Contains(err.Error(), "重新打开") {
		t.Fatalf("uncertain publication response = %v", err)
	}

	logger := useAPITestLog(t)
	result := persistence.DirectorySwitchResult{Committed: true, Warning: errors.New("pending cleanup warning")}
	if err := directorySwitchResponse("target", result, nil); err != nil {
		t.Fatalf("committed response = %v", err)
	}
	files, err := logger.List()
	if err != nil || len(files) != 1 {
		t.Fatalf("committed switch warning log = %v, %v", files, err)
	}
	contents, err := logger.Read(files[0].Name)
	if err != nil || !strings.Contains(contents, "pending cleanup warning") {
		t.Fatalf("committed switch warning = %q, %v", contents, err)
	}
}

// TestLinuxSetDataDirectorySerializesOrdinaryRequests 验证切换目录会串行化普通请求。
func TestLinuxSetDataDirectorySerializesOrdinaryRequests(t *testing.T) {
	api, target, _ := dataDirectoryBindingFixture(t)
	if _, err := testExec(api.app, `INSERT INTO subjects(name,created_at) VALUES('new connection','2026-11-08');
		CREATE TEMP TABLE subjects (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if got := api.Call("snapshot", http.MethodGet, ""); !strings.Contains(got, `"subjectsCount":0`) {
		t.Fatalf("old connection marker missing: %s", got)
	}

	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	borrow := make(chan error, 1)
	go func() {
		borrow <- api.app.database.WithRead(func(db *sql.DB, _ string) error {
			close(entered)
			<-release
			return db.Ping()
		})
	}()
	readAPIResult(t, entered)

	started, switched := make(chan string, 1), make(chan error, 1)
	go func() {
		started <- bindingGoroutineID()
		switched <- api.SetDataDirectory(target)
	}()
	waitForBindingSwitchWriteLock(t, readAPIResult(t, started), switched)
	assertStartupPathMissing(t, target)

	requestStarted, response := make(chan string, 1), make(chan string, 1)
	go func() {
		requestStarted <- bindingGoroutineID()
		response <- api.Call("snapshot", http.MethodGet, "")
	}()
	waitForAPIReadLock(t, readAPIResult(t, requestStarted), response)
	unblock()
	if err := readAPIResult(t, borrow); err != nil {
		t.Fatalf("switch closed the borrowed connection early: %v", err)
	}
	if err := readAPIResult(t, switched); err != nil {
		t.Fatal(err)
	}
	if got := readAPIResult(t, response); !strings.Contains(got, `"subjectsCount":1`) {
		t.Fatalf("queued request did not use the committed connection: %s", got)
	}
}

func bindingGoroutineID() string {
	var stack [128]byte
	n := runtime.Stack(stack[:], false)
	id, _, _ := strings.Cut(string(stack[:n]), " [")
	return id
}

func waitForBindingSwitchWriteLock(t *testing.T, id string, result <-chan error) {
	t.Helper()
	timeout := time.After(apiTestTimeout)
	stack := make([]byte, 64*1024)
	for {
		select {
		case err := <-result:
			t.Fatalf("switch returned before active borrow ended: %v", err)
		case <-timeout:
			t.Fatal("switch did not wait for the lifecycle write lock")
		default:
		}
		n := runtime.Stack(stack, true)
		if n == len(stack) {
			stack = make([]byte, 2*len(stack))
			continue
		}
		for _, entry := range strings.Split(string(stack[:n]), "\n\n") {
			if strings.HasPrefix(entry, id+" [sync.") && strings.Contains(entry, "sync.(*RWMutex).Lock(") &&
				strings.Contains(entry, "persistence.(*Database).SwitchDirectory(") {
				return
			}
		}
		runtime.Gosched()
	}
}
