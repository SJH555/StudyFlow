//go:build windows

package system

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValueName = "StudyFlow"

func studyFlowCommand() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	return `"` + executable + `"`, nil
}

// AutostartEnabled 报告当前用户是否已设置指向本程序的自启动项。
func AutostartEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(autostartValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	expected, err := studyFlowCommand()
	if err != nil {
		return false, err
	}
	return strings.EqualFold(value, expected), nil
}

// SetAutostart 启用或禁用当前用户的 StudyFlow 自启动项。
func SetAutostart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(autostartValueName)
	exists := err == nil
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	expected, err := studyFlowCommand()
	if err != nil {
		return err
	}
	if exists && !strings.EqualFold(value, expected) {
		return fmt.Errorf("StudyFlow 自启动项已有其他值，未修改")
	}
	if enabled {
		return key.SetStringValue(autostartValueName, expected)
	}
	if exists {
		return key.DeleteValue(autostartValueName)
	}
	return nil
}
