//go:build linux

package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const autostartMarker = "X-StudyFlow-Managed=true"

func autostartPath() (string, error) {
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "autostart", "org.wails.studyflow.desktop"), nil
}

func managedAutostartFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("自启动路径不是普通文件，未修改：%s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.Contains("\n"+string(data)+"\n", "\n"+autostartMarker+"\n"), nil
}

// AutostartEnabled 报告当前是否已设置 StudyFlow 自启动项。
func AutostartEnabled() (bool, error) {
	path, err := autostartPath()
	if err != nil {
		return false, err
	}
	return managedAutostartFile(path)
}

// SetAutostart 启用或禁用 StudyFlow 的自启动项。
func SetAutostart(enabled bool) error {
	path, err := autostartPath()
	if err != nil {
		return err
	}
	managed, err := managedAutostartFile(path)
	if err != nil {
		return err
	}
	if !enabled {
		if !managed {
			return nil
		}
		return os.Remove(path)
	}
	if !managed {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("自启动条目已存在且不属于 StudyFlow，未覆盖")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	executable := os.Getenv("APPIMAGE")
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	// Desktop Entry 字符串使用反斜杠转义，而不是 shell 引号。
	escaped := strings.NewReplacer("\\", "\\\\", " ", "\\s", "\t", "\\t", "\n", "\\n", "\"", "\\\"").Replace(executable)
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=StudyFlow\nExec=%s\nTryExec=%s\nTerminal=false\n%s\n", escaped, escaped, autostartMarker)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".studyflow-autostart-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
