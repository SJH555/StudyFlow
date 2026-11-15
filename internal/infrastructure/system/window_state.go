package system

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SavedWindowState 描述持久化的窗口尺寸与位置状态。
type SavedWindowState struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	X           int  `json:"x"`
	Y           int  `json:"y"`
	HasPosition bool `json:"hasPosition"`
	Maximised   bool `json:"maximised"`
}

// ReadWindowState 读取窗口状态文件；文件不存在时返回零值。
func ReadWindowState(path string) (SavedWindowState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SavedWindowState{}, nil
	}
	if err != nil {
		return SavedWindowState{}, err
	}
	var state SavedWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return SavedWindowState{}, err
	}
	return state, nil
}

func windowPositionAvailable(goos string) bool {
	if goos == "windows" {
		return true
	}
	if goos != "linux" {
		return false
	}
	backend := strings.ToLower(os.Getenv("GDK_BACKEND"))
	return backend == "x11" || (backend != "wayland" && strings.ToLower(os.Getenv("XDG_SESSION_TYPE")) == "x11")
}

// ApplyWindowState 将保存的状态应用到窗口选项，忽略越界或不可用的值。
func ApplyWindowState(options *application.WebviewWindowOptions, state SavedWindowState, goos string) {
	if state.Width >= options.MinWidth && state.Width <= 10000 && state.Height >= options.MinHeight && state.Height <= 10000 {
		options.Width = state.Width
		options.Height = state.Height
	}
	if state.HasPosition && windowPositionAvailable(goos) && state.X >= -20000 && state.X <= 20000 && state.Y >= -20000 && state.Y <= 20000 {
		options.X = state.X
		options.Y = state.Y
		options.InitialPosition = application.WindowXY
	}
	if state.Maximised {
		options.StartState = application.WindowStateMaximised
	}
}

// WindowStateTracker 防抖地捕获并持久化窗口状态。
type WindowStateTracker struct {
	mu        sync.Mutex
	path      string
	position  bool
	state     SavedWindowState
	timer     *time.Timer
	timerDone func()
	closed    bool
	captures  sync.WaitGroup
	closeOnce sync.Once
}

// NewWindowStateTracker 创建窗口状态跟踪器。
func NewWindowStateTracker(path string, goos string, previous SavedWindowState) *WindowStateTracker {
	return &WindowStateTracker{path: path, position: windowPositionAvailable(goos), state: previous}
}

// Schedule 安排一次延迟捕获，用于窗口变化后的防抖保存。
func (tracker *WindowStateTracker) Schedule(window *application.WebviewWindow) {
	tracker.schedule(func() { tracker.capture(window) })
}

func (tracker *WindowStateTracker) schedule(capture func()) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.closed {
		return
	}
	if tracker.timer != nil && tracker.timer.Stop() {
		tracker.timerDone()
	}
	tracker.captures.Add(1)
	var once sync.Once
	done := func() { once.Do(tracker.captures.Done) }
	tracker.timerDone = done
	tracker.timer = time.AfterFunc(350*time.Millisecond, func() {
		defer done()
		capture()
	})
}

// Close 在 Wails 主线程仍在分发窗口调用时执行。启动失败、原生窗口尚未运行时使用 nil 窗口。
func (tracker *WindowStateTracker) Close(window *application.WebviewWindow) {
	tracker.closeOnce.Do(func() {
		tracker.mu.Lock()
		tracker.closed = true
		if tracker.timer != nil && tracker.timer.Stop() {
			tracker.timerDone()
		}
		tracker.mu.Unlock()
		tracker.captures.Wait()
		if window != nil {
			tracker.capture(window)
		}
	})
}

// Capture 立即捕获当前窗口状态并保存。
func (tracker *WindowStateTracker) Capture(window *application.WebviewWindow) {
	tracker.mu.Lock()
	if tracker.closed {
		tracker.mu.Unlock()
		return
	}
	tracker.captures.Add(1)
	tracker.mu.Unlock()
	defer tracker.captures.Done()
	tracker.capture(window)
}

func (tracker *WindowStateTracker) capture(window *application.WebviewWindow) {
	if window.IsMinimised() || window.IsFullscreen() {
		return
	}
	if window.IsMaximised() {
		tracker.mu.Lock()
		tracker.state.Maximised = true
		if err := writeWindowState(tracker.path, tracker.state); err != nil {
			log.Printf("保存窗口状态失败: %v", err)
		}
		tracker.mu.Unlock()
		return
	}
	width, height := window.Size()
	if width < 1000 || height < 700 || width > 10000 || height > 10000 {
		return
	}
	state := SavedWindowState{Width: width, Height: height}
	if tracker.position {
		state.X, state.Y = window.Position()
		state.HasPosition = true
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.state = state
	if err := writeWindowState(tracker.path, state); err != nil {
		log.Printf("保存窗口状态失败: %v", err)
	}
}

func writeWindowState(path string, state SavedWindowState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".window-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
