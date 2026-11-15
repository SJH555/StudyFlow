package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func finalize(root, kind, platform, arch string) error {
	if platform != "linux" && platform != "windows" && platform != "darwin" {
		// 移动平台继续使用各自 Wails 任务的产物格式。
		return nil
	}
	if arch != "amd64" && arch != "arm64" && arch != "386" {
		return fmt.Errorf("不支持的产物架构: %s", arch)
	}
	version, err := readVersion(root)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "bin")
	prefix := "StudyFlow-" + version + "-" + platform + "-" + arch
	if kind == "binary" {
		suffix := ""
		if platform == "windows" {
			suffix = ".exe"
		}
		source := filepath.Join(dir, "StudyFlow"+suffix)
		target := filepath.Join(dir, prefix+suffix)
		if err := copyBinary(source, target); err != nil {
			return err
		}
		return removeOld(dir, "StudyFlow-*-"+platform+"-"+arch+suffix, target)
	}
	if platform == "linux" {
		for _, extension := range []string{"AppImage", "deb", "rpm", "pkg.tar.zst"} {
			target := filepath.Join(dir, prefix+"."+extension)
			if err := os.Rename(filepath.Join(dir, "StudyFlow."+extension), target); err != nil {
				return err
			}
			if err := removeOld(dir, "StudyFlow-*-linux-"+arch+"."+extension, target); err != nil {
				return err
			}
			// 同时清理旧命名规则下同架构的安装包，保留其他架构和平台的产物。
			legacyArchitectures := []string{arch}
			if arch == "amd64" {
				legacyArchitectures = append(legacyArchitectures, "x86_64")
			}
			for _, legacyArch := range legacyArchitectures {
				if err := removeOld(dir, "StudyFlow-*-"+legacyArch+"."+extension, target); err != nil {
					return err
				}
			}
			fmt.Println("安装包:", target)
		}
		return nil
	}
	if platform == "windows" {
		target := filepath.Join(dir, "StudyFlow-"+version+"-"+arch+"-installer.exe")
		if _, err := os.Stat(target); err == nil {
			return removeOld(dir, "StudyFlow-*-"+arch+"-installer.exe", target)
		} else if !os.IsNotExist(err) {
			return err
		}
		target = filepath.Join(dir, prefix+".msix")
		if err := os.Rename(filepath.Join(dir, "StudyFlow-"+arch+".msix"), target); err != nil {
			return err
		}
		return removeOld(dir, "StudyFlow-*-windows-"+arch+".msix", target)
	}
	// macOS 使用目录形式的应用包，保留 Wails 的标准目录名。
	return nil
}

func copyBinary(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target+".tmp", data, 0755); err != nil {
		return err
	}
	if err := os.Rename(target+".tmp", target); err != nil {
		return err
	}
	fmt.Println("可执行文件:", target)
	return nil
}

func removeOld(dir, pattern, keep string) error {
	paths, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if path == keep {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("拒绝清理非普通产物文件: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
