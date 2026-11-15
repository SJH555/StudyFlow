//go:build windows

package system

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const windowsWakeEventName = `Local\StudyFlow_ShowWindow`

type windowsInstanceGuard struct {
	mutex     windows.Handle
	wake      windows.Handle
	done      chan struct{}
	mu        sync.Mutex
	wg        sync.WaitGroup
	stopped   bool
	listening bool
	stopOnce  sync.Once
	closeOnce sync.Once
	closeErr  error
}

// AcquireSingleInstance 通过命名互斥体实现单实例；互斥体已存在时通知现有窗口并返回 false。
func AcquireSingleInstance(_ string) (InstanceGuard, bool, error) {
	name, err := windows.UTF16PtrFromString(`Local\StudyFlow_SingleInstance`)
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(handle)
		requestWindowsWindow()
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	wakeName, err := windows.UTF16PtrFromString(windowsWakeEventName)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, false, err
	}
	wake, err := windows.CreateEvent(nil, 0, 0, wakeName)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, false, err
	}
	return &windowsInstanceGuard{mutex: handle, wake: wake, done: make(chan struct{})}, true, nil
}

func requestWindowsWindow() {
	name, _ := windows.UTF16PtrFromString(windowsWakeEventName)
	for attempt := 0; attempt < 20; attempt++ {
		wake, err := windows.OpenEvent(0x0002, false, name) // EVENT_MODIFY_STATE：修改事件状态的访问权限
		if err == nil {
			_ = windows.SetEvent(wake)
			_ = windows.CloseHandle(wake)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (g *windowsInstanceGuard) ListenForSecondLaunch(show func()) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped || g.listening {
		return errors.New("single-instance listener unavailable")
	}
	g.listening = true
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			select {
			case <-g.done:
				return
			default:
			}
			result, err := windows.WaitForSingleObject(g.wake, 500)
			if err != nil {
				return
			}
			if result == windows.WAIT_OBJECT_0 {
				show()
			}
		}
	}()
	return nil
}

func (g *windowsInstanceGuard) StopListening() error {
	g.stopOnce.Do(func() {
		g.mu.Lock()
		g.stopped = true
		close(g.done)
		g.mu.Unlock()
		g.wg.Wait()
	})
	return nil
}

func (g *windowsInstanceGuard) Close() error {
	g.closeOnce.Do(func() {
		g.closeErr = errors.Join(g.StopListening(), windows.CloseHandle(g.wake), windows.CloseHandle(g.mutex))
	})
	return g.closeErr
}
