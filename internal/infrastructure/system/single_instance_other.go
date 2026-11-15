//go:build !linux && !windows

package system

import (
	"os"
	"path/filepath"
	"sync"
)

type otherInstanceGuard struct {
	lock      string
	closeOnce sync.Once
	closeErr  error
}

// AcquireSingleInstance 通过独占创建锁文件实现单实例；文件已存在时返回 false。
func AcquireSingleInstance(baseDir string) (InstanceGuard, bool, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, false, err
	}
	path := filepath.Join(baseDir, "StudyFlow.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	_ = f.Close()
	return &otherInstanceGuard{lock: path}, true, nil
}

func (g *otherInstanceGuard) ListenForSecondLaunch(func()) error { return nil }
func (g *otherInstanceGuard) StopListening() error               { return nil }
func (g *otherInstanceGuard) Close() error {
	g.closeOnce.Do(func() { g.closeErr = os.Remove(g.lock) })
	return g.closeErr
}
