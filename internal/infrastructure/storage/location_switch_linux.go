//go:build linux && !android

package storage

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func defaultLocationIO() locationIO {
	return locationIO{write: (*os.File).Write, syncFile: (*os.File).Sync, closeFile: (*os.File).Close,
		syncDir: (*os.File).Sync, rename: unix.Renameat2, moveOwned: unix.Renameat2, unlinkOwned: unix.Unlinkat}
}

func openLocationRead(dir *os.File, name string) (*os.File, error) {
	fd := unix.AT_FDCWD
	if dir != nil {
		fd = int(dir.Fd())
	}
	return openLocationAt(fd, name, unix.O_RDONLY, 0)
}

func createLocationFile(dir *os.File, name string) (*os.File, error) {
	return openLocationAt(int(dir.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0600)
}

func openLocationAt(dir int, name string, flags int, mode uint32) (*os.File, error) {
	fd, err := unix.Openat(dir, name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, mode)
	if err != nil {
		return nil, &os.PathError{Op: "open location file", Path: name, Err: err}
	}
	return os.NewFile(uintptr(fd), name), nil
}

func openLocationDirectory(path string) (*os.File, locationIdentity, error) {
	dir, err := openLocationAt(unix.AT_FDCWD, path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, locationIdentity{}, err
	}
	id, err := getLocationIdentity(dir, unix.S_IFDIR)
	if err != nil {
		return nil, locationIdentity{}, errors.Join(err, dir.Close())
	}
	return dir, id, nil
}

func getLocationIdentity(file *os.File, fileType uint32) (locationIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return locationIdentity{}, err
	}
	if uint32(stat.Mode)&unix.S_IFMT != fileType {
		return locationIdentity{}, fmt.Errorf("unexpected storage file type: %s", file.Name())
	}
	id := locationIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}
	var statx unix.Statx_t
	if err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_BTIME, &statx); err == nil && statx.Mask&unix.STATX_BTIME != 0 {
		id.Birth = fmt.Sprintf("%d.%09d", statx.Btime.Sec, statx.Btime.Nsec)
	}
	return id, nil
}
