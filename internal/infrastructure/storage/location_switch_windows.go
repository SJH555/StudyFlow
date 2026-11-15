package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func defaultLocationIO() locationIO {
	return locationIO{
		write: (*os.File).Write, syncFile: (*os.File).Sync, closeFile: (*os.File).Close,
		syncDir: syncWindowsLocationDirectory,
		rename:  renameWindowsLocation, moveOwned: renameWindowsLocation,
		unlinkOwned: removeWindowsLocation,
	}
}

func windowsLocationPath(dir int, name string) (string, error) {
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(dir), &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buffer)) {
		return "", errors.New("storage directory path is too long")
	}
	return filepath.Join(windows.UTF16ToString(buffer[:n]), name), nil
}

func openWindowsLocation(path string, access, creation, share uint32) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, access, share, nil, creation,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open storage location", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}

func openLocationRead(dir *os.File, name string) (*os.File, error) {
	path := name
	if dir != nil {
		var err error
		path, err = windowsLocationPath(int(dir.Fd()), name)
		if err != nil {
			return nil, err
		}
	}
	return openWindowsLocation(path, windows.GENERIC_READ, windows.OPEN_EXISTING, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
}

func createLocationFile(dir *os.File, name string) (*os.File, error) {
	path, err := windowsLocationPath(int(dir.Fd()), name)
	if err != nil {
		return nil, err
	}
	return openWindowsLocation(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.CREATE_NEW, windows.FILE_SHARE_READ)
}

func openLocationDirectory(path string) (*os.File, locationIdentity, error) {
	// 不共享删除权限，避免持有期间目录被重命名或替换；文件仍按各自身份复核。
	dir, err := openWindowsLocation(path, windows.GENERIC_READ, windows.OPEN_EXISTING, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
	if err != nil {
		return nil, locationIdentity{}, err
	}
	id, err := getLocationIdentity(dir, locationDirectory)
	if err != nil {
		return nil, locationIdentity{}, errors.Join(err, dir.Close())
	}
	return dir, id, nil
}

func getLocationIdentity(file *os.File, fileType uint32) (locationIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return locationIdentity{}, err
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || isDirectory != (fileType == locationDirectory) {
		return locationIdentity{}, fmt.Errorf("unexpected storage file type: %s", file.Name())
	}
	return locationIdentity{
		Device: uint64(info.VolumeSerialNumber),
		Inode:  uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
		// NTFS 文件隧道可能在重命名到近期删除的名称时恢复创建时间；身份只使用卷号与文件 ID。
	}, nil
}

func renameWindowsLocation(fromDir int, from string, toDir int, to string, flags uint) error {
	source, err := windowsLocationPath(fromDir, from)
	if err != nil {
		return err
	}
	target, err := windowsLocationPath(toDir, to)
	if err != nil {
		return err
	}
	oldName, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	newName, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	moveFlags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if flags == 0 {
		moveFlags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	return windows.MoveFileEx(oldName, newName, moveFlags)
}

func removeWindowsLocation(dir int, name string, _ int) error {
	path, err := windowsLocationPath(dir, name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func syncWindowsLocationDirectory(*os.File) error {
	// Windows 不提供等价于 Linux fsync(dir) 的普通目录刷新。
	// 文件先 FlushFileBuffers，再用 MOVEFILE_WRITE_THROUGH 发布，并由公共协议回读确认。
	return nil
}
