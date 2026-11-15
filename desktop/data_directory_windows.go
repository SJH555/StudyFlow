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
	return filepath.Abs(filepath.Dir(executable))
}
