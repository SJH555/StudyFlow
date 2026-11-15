package persistence

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupFile 描述一个备份文件及其解析出的时间戳。
type BackupFile struct {
	Name      string
	Timestamp time.Time
}

// ListBackups 列出数据库备份文件；createDir 为 true 时先确保备份目录存在。
func ListBackups(dbPath string, createDir bool) ([]BackupFile, error) {
	dir := backupDirectory(dbPath)
	if createDir {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []BackupFile
	for _, entry := range entries {
		if stamp, ok := backupTimestamp(entry.Name()); ok && !entry.IsDir() {
			files = append(files, BackupFile{Name: entry.Name(), Timestamp: stamp})
		}
	}
	return files, nil
}

// CreateBackup 通过 SQLite 在线备份在当前数据库旁生成一份带时间戳的备份。
func CreateBackup(db *sql.DB, dbPath string, now time.Time) error {
	dir := backupDirectory(dbPath)
	name := "study-" + now.Format("20060102-150405") + ".db"
	target := filepath.Join(dir, name)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.%d.tmp", name, now.UnixNano()))
	defer os.Remove(tmp)
	if _, err := db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// RemoveExpiredBackups 仅清理调用方提供的列表中早于 cutoff 的备份文件。
func RemoveExpiredBackups(dbPath string, files []BackupFile, cutoff time.Time) error {
	dir := backupDirectory(dbPath)
	for _, file := range files {
		if !file.Timestamp.After(cutoff) {
			if err := os.Remove(filepath.Join(dir, file.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func backupDirectory(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

func backupTimestamp(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, "study-") || !strings.HasSuffix(name, ".db") {
		return time.Time{}, false
	}
	stamp, err := time.ParseInLocation("20060102-150405", strings.TrimSuffix(strings.TrimPrefix(name, "study-"), ".db"), time.Local)
	return stamp, err == nil
}
