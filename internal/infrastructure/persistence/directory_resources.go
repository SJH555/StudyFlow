package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (p *preparedDirectory) makeDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("目标路径不是普通目录: %s", path)
		}
		return checkCanonicalDirectory(path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := p.makeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	_, err := p.recordDirectory(path)
	return err
}

func (p *preparedDirectory) recordDirectory(path string) (preparedResource, error) {
	info, err := openedDirectoryInfo(path)
	resource := preparedResource{path: path, info: info}
	p.resources = append(p.resources, resource)
	return resource, err
}

// openedDirectoryInfo 从打开的句柄捕获目录身份。Windows 上基于路径的 FileInfo 会延迟到首次比较时才读取文件 ID。
func openedDirectoryInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("本次创建的目录已被替换: %s", path)
	}
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := dir.Stat()
	closeErr := dir.Close()
	if err == nil && !os.SameFile(info, opened) {
		return nil, errors.Join(fmt.Errorf("本次创建的目录身份已变化: %s", path), closeErr)
	}
	return opened, errors.Join(err, closeErr)
}

// openedDatabaseInfo 固定句柄身份，避免 Windows 的路径型 FileInfo 延迟读取文件 ID。
func openedDatabaseInfo(path string) (os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("源数据库不是普通文件: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil && !os.SameFile(before, opened) {
		err = fmt.Errorf("源数据库身份已变化: %s", path)
	}
	return opened, errors.Join(err, file.Close())
}

func checkCanonicalDirectory(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(path, resolved); err != nil || rel != "." {
		return fmt.Errorf("目录路径已变化: %s", path)
	}
	return nil
}

func (r preparedResource) checkIdentity() error {
	_, err := r.currentInfo()
	return err
}

func (r preparedResource) currentInfo() (os.FileInfo, error) {
	if err := checkCanonicalDirectory(filepath.Dir(r.path)); err != nil {
		return nil, err
	}
	current, err := os.Lstat(r.path)
	if err != nil {
		return nil, err
	}
	if r.info == nil || !os.SameFile(r.info, current) {
		return nil, fmt.Errorf("文件身份无法确认，已保留: %s", r.path)
	}
	if r.checkMetadata && (r.info.Size() != current.Size() || !r.info.ModTime().Equal(current.ModTime())) {
		return nil, fmt.Errorf("文件内容可能已被修改，已保留: %s", r.path)
	}
	return current, nil
}

func (r preparedResource) syncFile() error {
	if err := r.checkIdentity(); err != nil {
		return err
	}
	file, err := os.OpenFile(r.path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err == nil && !os.SameFile(r.info, info) {
		err = fmt.Errorf("文件身份已变化: %s", r.path)
	}
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

func (r preparedResource) remove() error {
	if err := r.checkIdentity(); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理准备资源 %s: %w", r.path, err)
	}
	return nil
}

func (p *preparedDirectory) cleanup() error {
	var result error
	for i := len(p.resources) - 1; i >= 0; i-- {
		result = errors.Join(result, p.resources[i].remove())
	}
	return result
}
