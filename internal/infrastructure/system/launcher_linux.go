//go:build linux

package system

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// EnsureUserLauncher 仅为已安装的包（位于 /usr/bin 或 /usr/local/bin）在用户 XDG 数据目录注册启动器与图标；开发态二进制不注册。
func EnsureUserLauncher(executable string, icon []byte) error {
	executable = filepath.Clean(executable)
	if executable != "/usr/local/bin/StudyFlow" && executable != "/usr/bin/StudyFlow" {
		return nil
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(dataHome) {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	iconPath := filepath.Join(dataHome, "icons", "hicolor", "1024x1024", "apps", "org.wails.studyflow.png")
	desktopPath := filepath.Join(dataHome, "applications", "org.wails.studyflow.desktop")
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=StudyFlow
Comment=学习时间管理与记录
Exec=%s
TryExec=%s
Icon=%s
Categories=Education;
Terminal=false
StartupNotify=true
StartupWMClass=org.wails.studyflow
`, executable, executable, iconPath)
	for _, file := range []struct {
		path string
		data []byte
	}{{iconPath, icon}, {desktopPath, []byte(desktop)}} {
		if err := os.MkdirAll(filepath.Dir(file.path), 0755); err != nil {
			return err
		}
		if existing, err := os.ReadFile(file.path); err == nil && bytes.Equal(existing, file.data) {
			continue
		}
		tmp, err := os.CreateTemp(filepath.Dir(file.path), ".studyflow-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if err := tmp.Chmod(0644); err != nil {
			tmp.Close()
			return err
		}
		if _, err := tmp.Write(file.data); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), file.path); err != nil {
			return err
		}
	}
	return nil
}
