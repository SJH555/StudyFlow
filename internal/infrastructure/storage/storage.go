package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BaseDir 根据操作系统返回数据基目录：Linux 下为 ~/.StudyFlow，其余平台为安装目录。
func BaseDir(goos, installDir, homeDir string) string {
	if goos == "linux" {
		return filepath.Join(homeDir, ".StudyFlow")
	}
	return installDir
}

// StartupData 描述只读位置选择的结果。
type StartupData struct {
	DatabasePath     string
	LegacySource     string
	PromoteDirectory string
	Create           bool
}

// SelectStartupData 应用历史位置优先级，但不复制文件或提升旧位置。
func SelectStartupData(locationFile, defaultDir, installDir, legacyConfigDir string) (StartupData, error) {
	chosen, err := ReadStorageLocation(locationFile)
	if err != nil {
		return StartupData{}, err
	}
	promotion := ""
	if chosen == "" {
		for _, oldFile := range []string{
			filepath.Join(installDir, "storage-location.txt"),
			filepath.Join(legacyConfigDir, "storage-location.txt"),
		} {
			if filepath.Clean(oldFile) == filepath.Clean(locationFile) {
				continue
			}
			chosen, err = ReadStorageLocation(oldFile)
			if err != nil {
				return StartupData{}, err
			}
			if chosen != "" && filepath.Clean(chosen) != filepath.Clean(installDir) {
				break
			}
			chosen = ""
		}
		if chosen != "" && filepath.Clean(chosen) != filepath.Clean(defaultDir) {
			promotion = chosen
		}
	}
	if chosen != "" && filepath.Clean(chosen) != filepath.Clean(installDir) {
		return StartupData{DatabasePath: filepath.Join(chosen, "study.db"), PromoteDirectory: promotion}, nil
	}

	result := StartupData{DatabasePath: filepath.Join(defaultDir, "study.db")}
	if _, err := os.Lstat(result.DatabasePath); err == nil {
		return result, nil
	} else if !os.IsNotExist(err) {
		return StartupData{}, err
	}
	for _, oldDB := range []string{
		filepath.Join(installDir, "study.db"),
		filepath.Join(legacyConfigDir, "study.db"),
	} {
		if filepath.Clean(oldDB) == filepath.Clean(result.DatabasePath) {
			continue
		}
		if _, err := os.Lstat(oldDB); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return StartupData{}, err
		}
		result.LegacySource = oldDB
		return result, nil
	}
	result.Create = true
	return result, nil
}

// ResolveDataDirectory 保留历史路径解析 API。启动流程使用 SelectStartupData，以便在数据库校验成功后再执行提升。
func ResolveDataDirectory(locationFile, defaultDir, installDir, legacyConfigDir string) (string, error) {
	result, err := SelectStartupData(locationFile, defaultDir, installDir, legacyConfigDir)
	if err != nil {
		return "", err
	}
	if result.PromoteDirectory != "" {
		if err := PromoteStartupLocation(locationFile, result.PromoteDirectory); err != nil {
			return "", err
		}
	}
	if result.LegacySource != "" {
		if err := CopyFile(result.LegacySource, result.DatabasePath); err != nil {
			return "", fmt.Errorf("迁移原有学习数据失败: %w", err)
		}
	}
	return filepath.Dir(result.DatabasePath), nil
}

// PromoteStartupLocation 以原始格式保存一个已验证的旧位置选择。
func PromoteStartupLocation(locationFile, directory string) error {
	return os.WriteFile(locationFile, []byte(directory+"\n"), 0600)
}

// ReadStorageLocation 读取存储位置文件的首行内容。
func ReadStorageLocation(path string) (string, error) {
	// 区分「条目不存在」与「存在但不可读」（含悬空链接）。真正为空的首行仍走旧版回退。
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read storage location %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("read storage location %s: %w", path, err)
		}
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("storage location is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read storage location %s: %w", path, err)
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line), nil
}

func hasStartupPending(locationFile, pendingName, cleanupSuffix string) (bool, error) {
	for _, name := range []string{pendingName, pendingName + cleanupSuffix} {
		path := filepath.Join(filepath.Dir(locationFile), name)
		if _, err := os.Lstat(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("check pending storage location %s: %w", path, err)
		}
	}
	return false, nil
}

// CopyFile 将源文件复制到目标，目标必须不存在。
func CopyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return closeErr
	}
	return nil
}
