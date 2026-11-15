package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type directoryPaths struct {
	target, sourceBackups, targetBackups string
	sameDirectory                        bool
}

func checkDirectoryPaths(sourceDB, target, locationFile string) (directoryPaths, error) {
	var paths directoryPaths
	if strings.TrimSpace(target) == "" || strings.ContainsAny(target, "\x00\r\n") {
		return paths, errors.New("请选择一个有效的数据目录")
	}
	sourceDir, err := resolveDirectoryPath(filepath.Dir(sourceDB))
	if err != nil {
		return paths, err
	}
	sourceInfo, err := os.Lstat(filepath.Join(sourceDir, filepath.Base(sourceDB)))
	if err != nil {
		return paths, err
	}
	if !sourceInfo.Mode().IsRegular() {
		return paths, fmt.Errorf("源数据库不是普通文件: %s", sourceDB)
	}
	paths.target, err = resolveDirectoryPath(target)
	if err != nil {
		return paths, err
	}
	sourceDirInfo, err := os.Stat(sourceDir)
	if err != nil {
		return paths, err
	}
	if targetInfo, err := os.Stat(paths.target); err == nil && os.SameFile(sourceDirInfo, targetInfo) {
		paths.sameDirectory = true
		return paths, nil
	} else if err != nil && !os.IsNotExist(err) {
		return paths, err
	}
	paths.sourceBackups = filepath.Join(sourceDir, "backups")
	paths.targetBackups = filepath.Join(paths.target, "backups")
	for _, path := range []string{paths.sourceBackups, paths.targetBackups} {
		if info, err := os.Lstat(path); err == nil && !info.IsDir() {
			return paths, fmt.Errorf("备份路径不是普通目录: %s", path)
		} else if err != nil && !os.IsNotExist(err) {
			return paths, err
		}
	}
	for _, pair := range [][2]string{{paths.sourceBackups, paths.target}, {paths.targetBackups, sourceDir}} {
		for i := range pair {
			contains, err := directoryContains(pair[i], pair[1-i])
			if err != nil {
				return paths, err
			}
			if contains {
				return paths, fmt.Errorf("数据与备份目录重叠: %s, %s", pair[0], pair[1])
			}
		}
	}
	locationDir, err := resolveDirectoryPath(filepath.Dir(locationFile))
	if err != nil {
		return paths, err
	}
	for _, dir := range []string{paths.sourceBackups, paths.targetBackups} {
		contains, err := directoryContains(dir, locationDir)
		if err != nil {
			return paths, err
		}
		if contains {
			return paths, fmt.Errorf("备份目录包含存储位置文件: %s", locationFile)
		}
	}
	locationPath := filepath.Join(locationDir, filepath.Base(locationFile))
	for _, name := range []string{"study.db", "study.db-wal", "study.db-shm", "study.db-journal"} {
		path := filepath.Join(paths.target, name)
		if rel, err := filepath.Rel(path, locationPath); err == nil && rel == "." {
			return paths, fmt.Errorf("目标数据库路径与存储位置文件冲突: %s", path)
		}
		if _, err := os.Lstat(path); err == nil {
			if name == "study.db" {
				return paths, errors.New("目标目录中已存在 study.db，请选择空目录")
			}
			return paths, fmt.Errorf("目标目录中已存在 SQLite 辅助文件: %s", path)
		} else if !os.IsNotExist(err) {
			return paths, err
		}
	}
	return paths, nil
}

// resolveDirectoryPath 解析已存在的祖先目录，即使最终目录尚未创建。
func resolveDirectoryPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err == nil {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			info, err = os.Stat(resolved)
			if err != nil {
				return "", err
			}
		}
		if !info.IsDir() {
			return "", fmt.Errorf("路径不是目录: %s", path)
		}
		return resolved, nil
	}
	parent := filepath.Dir(path)
	if !os.IsNotExist(err) || parent == path {
		return "", err
	}
	resolved, err := resolveDirectoryPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

// directoryContains 比较目录身份，同时处理尚未创建目录的路径关系。
func directoryContains(parent, path string) (bool, error) {
	info, err := os.Stat(parent)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for {
		if rel, err := filepath.Rel(parent, path); err == nil && rel == "." {
			return true, nil
		}
		if info != nil {
			candidate, err := os.Stat(path)
			if err != nil && !os.IsNotExist(err) {
				return false, err
			}
			if candidate != nil && os.SameFile(info, candidate) {
				return true, nil
			}
		}
		next := filepath.Dir(path)
		if next == path {
			return false, nil
		}
		path = next
	}
}
