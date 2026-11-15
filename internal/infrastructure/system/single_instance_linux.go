//go:build linux

package system

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type linuxInstanceGuard struct {
	lock      *os.File
	listener  net.Listener
	socket    string
	mu        sync.Mutex
	wg        sync.WaitGroup
	stopped   bool
	stopOnce  sync.Once
	stopErr   error
	closeOnce sync.Once
	closeErr  error
}

// AcquireSingleInstance 尝试获取单实例锁；成功时返回 true，已被占用时通知现有窗口并返回 false。
func AcquireSingleInstance(baseDir string) (InstanceGuard, bool, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, false, err
	}
	lock, err := os.OpenFile(filepath.Join(baseDir, "StudyFlow.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			requestExistingWindow(filepath.Join(baseDir, "StudyFlow.sock"))
			return nil, false, nil
		}
		return nil, false, err
	}
	return &linuxInstanceGuard{lock: lock, socket: filepath.Join(baseDir, "StudyFlow.sock")}, true, nil
}

func requestExistingWindow(socket string) {
	for attempt := 0; attempt < 20; attempt++ {
		conn, err := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if err == nil {
			_, _ = conn.Write([]byte("show\n"))
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (g *linuxInstanceGuard) ListenForSecondLaunch(show func()) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return errors.New("single-instance listener is stopped")
	}
	if g.listener != nil {
		return errors.New("single-instance listener already started")
	}
	_ = os.Remove(g.socket)
	listener, err := net.Listen("unix", g.socket)
	if err != nil {
		return err
	}
	g.listener = listener
	if err := os.Chmod(g.socket, 0600); err != nil {
		_ = listener.Close()
		return err
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var request [5]byte
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			n, _ := conn.Read(request[:])
			_ = conn.Close()
			if string(request[:n]) == "show\n" {
				show()
			}
		}
	}()
	return nil
}

func (g *linuxInstanceGuard) StopListening() error {
	g.stopOnce.Do(func() {
		g.mu.Lock()
		g.stopped = true
		if g.listener != nil {
			g.stopErr = g.listener.Close()
			if errors.Is(g.stopErr, net.ErrClosed) {
				g.stopErr = nil
			}
			if err := os.Remove(g.socket); err != nil && !os.IsNotExist(err) {
				g.stopErr = errors.Join(g.stopErr, err)
			}
		}
		g.mu.Unlock()
		g.wg.Wait()
	})
	return g.stopErr
}

func (g *linuxInstanceGuard) Close() error {
	g.closeOnce.Do(func() {
		g.closeErr = errors.Join(g.StopListening(), syscall.Flock(int(g.lock.Fd()), syscall.LOCK_UN), g.lock.Close())
	})
	return g.closeErr
}
