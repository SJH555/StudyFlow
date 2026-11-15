//go:build !linux && !windows

package system

import "fmt"

// AutostartEnabled 在非 Linux/Windows 平台上始终返回未启用。
func AutostartEnabled() (bool, error) { return false, nil }

// SetAutostart 在非 Linux/Windows 平台上返回错误。
func SetAutostart(bool) error { return fmt.Errorf("当前系统不支持自动启动") }
