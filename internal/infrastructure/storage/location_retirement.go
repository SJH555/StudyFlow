//go:build (linux && !android) || windows

package storage

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrRetirementCleanup 表示新位置已提交，但旧文件清理需要保留记录后重试。
var ErrRetirementCleanup = errors.New("retired storage cleanup is incomplete")

var errRetirementLocationChanged = errors.New("committed storage location changed during retirement")

type retiredBackup struct {
	Name     string           `json:"name"`
	ID       locationIdentity `json:"identity"`
	TargetID locationIdentity `json:"targetIdentity"`
	Hash     []byte           `json:"sha256"`
}

type locationRetirement struct {
	DirectoryID locationIdentity `json:"directoryId"`
	BackupsID   locationIdentity `json:"backupsId,omitzero"`
	Backups     []retiredBackup  `json:"backups,omitempty"`
}

func (r locationRetirement) valid() bool {
	if r.DirectoryID.Inode == 0 || (len(r.Backups) > 0 && r.BackupsID.Inode == 0) {
		return false
	}
	names := make(map[string]bool)
	for _, backup := range r.Backups {
		if filepath.Base(backup.Name) != backup.Name || !strings.HasSuffix(backup.Name, ".db") ||
			backup.ID.Inode == 0 || backup.TargetID.Inode == 0 || len(backup.Hash) != sha256.Size || names[backup.Name] {
			return false
		}
		names[backup.Name] = true
	}
	return true
}

// PlanRetirement 声明本次实际复制的备份。Stage 会复核两端内容并将所有权记录持久化。
func (s *LocationSwitch) PlanRetirement(backups []string) {
	s.retirementPaths = append([]string{}, backups...)
}

func locationDirectoryIdentity(path string) (locationIdentity, error) {
	dir, id, err := openLocationDirectory(path)
	if err != nil {
		return locationIdentity{}, err
	}
	return id, dir.Close()
}

func planLocationRetirement(old, target locationDatabase, paths []string) (*locationRetirement, error) {
	id, err := locationDirectoryIdentity(filepath.Dir(old.Path))
	if err != nil {
		return nil, err
	}
	plan := &locationRetirement{DirectoryID: id}
	for _, path := range paths {
		if filepath.Dir(path) != filepath.Join(filepath.Dir(old.Path), "backups") {
			return nil, fmt.Errorf("backup source is outside the retired directory: %s", path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		// 快照允许读取链接指向的普通备份，迁移后保留原链接及其目标。
		if !info.Mode().IsRegular() {
			continue
		}
		backup, err := readRetiredBackup(path)
		if err != nil {
			return nil, err
		}
		copy, err := readRetiredBackup(filepath.Join(filepath.Dir(target.Path), "backups", backup.Name))
		if err != nil || !bytes.Equal(copy.Hash, backup.Hash) {
			return nil, errors.Join(err, fmt.Errorf("backup changed after copying: %s", path))
		}
		backup.TargetID = copy.ID
		plan.Backups = append(plan.Backups, backup)
	}
	if len(plan.Backups) > 0 {
		plan.BackupsID, err = locationDirectoryIdentity(filepath.Join(filepath.Dir(old.Path), "backups"))
	}
	return plan, err
}

func readRetiredBackup(path string) (retiredBackup, error) {
	file, err := openLocationRead(nil, path)
	if err != nil {
		return retiredBackup{}, err
	}
	id, err := getLocationIdentity(file, locationRegular)
	hash := sha256.New()
	if err == nil {
		_, err = io.Copy(hash, file)
	}
	return retiredBackup{Name: filepath.Base(path), ID: id, Hash: hash.Sum(nil)}, errors.Join(err, file.Close())
}

// CleanupRetired 只能在确认提交且旧 SQLite 连接关闭后调用；启动恢复须先验证新库。
// 每个待删名称先隔离再复核，记录保留至所有删除持久化，支持进程中断后继续清理。
func (s *LocationSwitch) CleanupRetired() (resultErr error) {
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, errRetirementLocationChanged) {
			resultErr = errors.Join(ErrRetirementCleanup, resultErr)
		}
	}()
	if s.settled != LocationCommitted || s.record == nil {
		return errors.New("cannot clean an uncommitted storage location")
	}
	plan := s.record.Retirement
	if plan == nil {
		// 历史记录没有清理清单，沿用旧版本的保留行为。
		return nil
	}
	if err := s.confirmRetirement(); err != nil {
		return err
	}
	oldDir := filepath.Dir(s.record.OldDatabase.Path)
	if err := checkRetiredDirectory(oldDir, plan.DirectoryID); err != nil {
		return err
	}
	if err := s.checkRetiredDatabase(); err != nil {
		return err
	}
	// SQLite 负责 checkpoint 和移除 sidecar；清理协议绝不自行删除这些文件。
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(s.record.OldDatabase.Path + suffix); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, fmt.Errorf("retired SQLite sidecar remains: %s", suffix))
		}
	}
	if len(plan.Backups) > 0 {
		backupsDir := filepath.Join(oldDir, "backups")
		if err := checkRetiredDirectory(backupsDir, plan.BackupsID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		} else if err == nil {
			for _, backup := range plan.Backups {
				if err := s.checkRetiredBackupCopy(backupsDir, backup); err != nil {
					return err
				}
				if err := s.removeRetiredFile(backupsDir, backup.Name, backup.ID, backup.Hash); err != nil {
					return err
				}
			}
		}
	}
	if err := s.removeRetiredFile(oldDir, filepath.Base(s.record.OldDatabase.Path), s.record.OldDatabase.ID, nil); err != nil {
		return err
	}
	// 只尝试删除已确认身份的空备份目录，用户文件和子目录不在清理范围内。
	if len(plan.Backups) > 0 {
		path := filepath.Join(oldDir, "backups")
		if err := checkRetiredDirectory(path, plan.BackupsID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return s.syncRetiredDirectory(oldDir)
			}
			return err
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return s.syncRetiredDirectory(oldDir)
}

func (s *LocationSwitch) checkRetiredBackupCopy(sourceDir string, backup retiredBackup) error {
	remaining := false
	for _, name := range []string{backup.Name, ".studyflow-retired-" + s.record.OperationID + "-" + backup.Name} {
		if _, err := os.Lstat(filepath.Join(sourceDir, name)); err == nil {
			remaining = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !remaining {
		return nil
	}
	path := filepath.Join(filepath.Dir(s.record.TargetDatabase.Path), "backups", backup.Name)
	copy, err := readRetiredBackup(path)
	if err != nil || copy.ID != backup.TargetID || !bytes.Equal(copy.Hash, backup.Hash) {
		return errors.Join(err, fmt.Errorf("migrated backup changed before retirement: %s", path))
	}
	return nil
}

func checkRetiredDirectory(path string, expected locationIdentity) error {
	id, err := locationDirectoryIdentity(path)
	if err != nil || id != expected {
		return errors.Join(err, fmt.Errorf("retired directory identity changed: %s", path))
	}
	return nil
}

func (s *LocationSwitch) confirmRetirement() error {
	result, err := s.observe(false)
	if err != nil || result.Outcome != LocationCommitted {
		return errors.Join(err, errRetirementLocationChanged)
	}
	return nil
}

func (s *LocationSwitch) checkRetiredDatabase() error {
	path := s.record.OldDatabase.Path
	isolated := filepath.Join(filepath.Dir(path), ".studyflow-retired-"+s.record.OperationID+"-"+filepath.Base(path))
	var found int
	for _, name := range []string{path, isolated} {
		file, err := openLocationRead(nil, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		id, err := getLocationIdentity(file, locationRegular)
		closeErr := file.Close()
		if err != nil || closeErr != nil || id != s.record.OldDatabase.ID {
			return errors.Join(err, closeErr, fmt.Errorf("retired database identity changed: %s", name))
		}
		found++
	}
	if found > 1 {
		return errors.New("both retired database names are occupied")
	}
	return nil
}

func (s *LocationSwitch) removeRetiredFile(dirPath, name string, id locationIdentity, hash []byte) (resultErr error) {
	if err := s.confirmRetirement(); err != nil {
		return err
	}
	dir, directoryID, err := openLocationDirectory(dirPath)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, dir.Close()) }()
	expected := s.record.Retirement.DirectoryID
	if dirPath == filepath.Join(filepath.Dir(s.record.OldDatabase.Path), "backups") {
		expected = s.record.Retirement.BackupsID
	}
	if directoryID != expected {
		return fmt.Errorf("retired directory identity changed: %s", dirPath)
	}
	isolated := ".studyflow-retired-" + s.record.OperationID + "-" + name
	check := func(path string) (bool, error) {
		file, err := openLocationRead(dir, path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		current, err := getLocationIdentity(file, locationRegular)
		if err == nil && current != id {
			err = fmt.Errorf("retired file identity changed: %s", filepath.Join(dirPath, path))
		}
		if err == nil && hash != nil {
			digest := sha256.New()
			_, err = io.Copy(digest, file)
			if err == nil && !bytes.Equal(hash, digest.Sum(nil)) {
				err = fmt.Errorf("retired backup content changed: %s", path)
			}
		}
		return true, errors.Join(err, file.Close())
	}
	owned, err := check(isolated)
	if err != nil {
		return err
	}
	source, err := check(name)
	if err != nil {
		return err
	}
	fd := int(dir.Fd())
	if owned && source {
		return fmt.Errorf("both retired file names are occupied: %s", name)
	}
	if !owned && source {
		if err := s.io.moveOwned(fd, name, fd, isolated, locationNoReplace); err != nil {
			return err
		}
		owned, err = check(isolated)
		if err != nil || !owned {
			return errors.Join(err, fmt.Errorf("cannot confirm isolated retired file: %s", isolated))
		}
	}
	if owned {
		if err := s.io.unlinkOwned(fd, isolated, 0); err != nil {
			return err
		}
	}
	return s.io.syncDir(dir)
}

func (s *LocationSwitch) syncRetiredDirectory(path string) error {
	dir, _, err := openLocationDirectory(path)
	if err != nil {
		return err
	}
	return errors.Join(s.io.syncDir(dir), dir.Close())
}
