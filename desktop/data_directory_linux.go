//go:build linux && !android

package main

import (
	"os"
	"path/filepath"
)

func dataDirectoryInstallDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	// 与启动阶段对历史安装标记的解读保持一致。
	if appImage := os.Getenv("APPIMAGE"); appImage != "" {
		executable = appImage
	}
	return filepath.Abs(filepath.Dir(executable))
}
