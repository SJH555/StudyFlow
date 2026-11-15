package diagnostics

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// Logger 按天管理错误日志文件，写入时自动轮转并清理过期日志。
type Logger struct {
	mu        sync.Mutex
	dir       string
	day       string
	file      *os.File
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
	closed    bool
}

// MigrateLegacyLogs 将旧版按天错误日志追加合并到目标目录，成功后删除源文件与源目录。
func MigrateLegacyLogs(sourceDir, targetDir string) error {
	entries, err := os.ReadDir(sourceDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = os.MkdirAll(targetDir, 0700); err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "error-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "error-"), ".log")
		if _, err = time.Parse("2006-01-02", day); err != nil {
			continue
		}
		source := filepath.Join(sourceDir, name)
		in, openErr := os.Open(source)
		if openErr != nil {
			return openErr
		}
		out, openErr := os.OpenFile(filepath.Join(targetDir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if openErr != nil {
			_ = in.Close()
			return openErr
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err = os.Remove(source); err != nil {
			return err
		}
	}
	_ = os.Remove(sourceDir)
	return nil
}

// StartErrorLog 启动按天轮转的错误日志，立即创建当天文件并注册崩溃输出。
func StartErrorLog(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	l := &Logger{dir: dir, stop: make(chan struct{}), done: make(chan struct{})}
	if err := l.rotate(); err != nil {
		if l.file != nil {
			err = errors.Join(err, debug.SetCrashOutput(nil, debug.CrashOptions{}), l.file.Close())
		}
		return nil, err
	}
	go func() {
		defer close(l.done)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-ticker.C:
				_ = l.rotate()
			}
		}
	}()
	return l, nil
}

func (l *Logger) rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return os.ErrClosed
	}
	day := time.Now().Format("2006-01-02")
	if day != l.day {
		file, err := os.OpenFile(filepath.Join(l.dir, "error-"+day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if err = debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
			_ = file.Close()
			return err
		}
		old := l.file
		l.file, l.day = file, day
		if old != nil {
			_ = old.Close()
		}
	}
	return l.pruneLocked()
}

func (l *Logger) pruneLocked() error {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "error-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "error-"), ".log")
		if _, parseErr := time.Parse("2006-01-02", day); parseErr == nil && day < cutoff {
			_ = os.Remove(filepath.Join(l.dir, name))
		}
	}
	return nil
}

// Write 实现 io.Writer，先按天轮转再写入当前日志文件。
func (l *Logger) Write(p []byte) (int, error) {
	if err := l.rotate(); err != nil {
		return 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	return l.file.Write(p)
}

// Close 停止日志轮转并关闭当前文件，重复调用安全。
func (l *Logger) Close() error {
	l.closeOnce.Do(func() {
		close(l.stop)
		<-l.done
		l.mu.Lock()
		defer l.mu.Unlock()
		l.closed = true
		l.closeErr = debug.SetCrashOutput(nil, debug.CrashOptions{})
		if l.file != nil {
			l.closeErr = errors.Join(l.closeErr, l.file.Close())
		}
	})
	return l.closeErr
}

// File 描述一个错误日志文件的元信息。
type File struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// List 返回当前目录下非空错误日志文件列表，按名称倒序排列。
func (l *Logger) List() ([]File, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}
	out := []File{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "error-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Size() > 0 {
			out = append(out, File{Name: entry.Name(), Size: info.Size(), Modified: info.ModTime().Format(time.RFC3339)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Read 读取指定名称的错误日志，内容上限 128 KiB，并拒绝路径穿越。
func (l *Logger) Read(name string) (string, error) {
	if filepath.Base(name) != name || !strings.HasPrefix(name, "error-") || !strings.HasSuffix(name, ".log") {
		return "", errors.New("invalid log file")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(filepath.Join(l.dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 128*1024))
	return string(b), err
}

// Directory 返回日志目录路径。
func (l *Logger) Directory() string { return l.dir }
