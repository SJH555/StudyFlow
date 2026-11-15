//go:build windows

package persistence

import "context"

// SwitchDirectory 在同一个生命周期写锁下发布新位置并替换当前句柄。
func (d *Database) SwitchDirectory(ctx context.Context, targetDir, locationFile, installDir string) (DirectorySwitchResult, error) {
	return d.switchDirectory(ctx, targetDir, locationFile, installDir, defaultDirectorySwitchOps())
}
