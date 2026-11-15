package persistence

import "golang.org/x/sys/windows"

func publishPreparedFile(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, target, windows.MOVEFILE_WRITE_THROUGH)
}

func syncPreparedDirectory(path string) error {
	// 文件已逐一刷新并通过 MOVEFILE_WRITE_THROUGH 发布；Windows 没有 Linux 的 fsync(dir)。
	// 此处复核目录，位置发布另有持久化记录与启动恢复，不能声称具备同等的断电保证。
	_, err := openedDirectoryInfo(path)
	return err
}
