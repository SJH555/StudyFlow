//go:build !linux

package system

// EnsureUserLauncher 在非 Linux 平台为空实现。
func EnsureUserLauncher(string, []byte) error { return nil }
