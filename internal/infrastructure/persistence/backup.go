package persistence

import (
	"context"
	"database/sql"
	"errors"
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
	return CreateBackupContext(context.Background(), db, dbPath, now)
}

// CreateBackupContext runs outside a SQL transaction under the owner's lifecycle
// lock. A failed/cancelled snapshot never replaces an existing backup.
func CreateBackupContext(ctx context.Context, db *sql.DB, dbPath string, now time.Time) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := backupDirectory(dbPath)
	name := "study-" + now.Format("20060102-150405") + ".db"
	target := filepath.Join(dir, name)
	return createBackupFile(ctx, db, target, now.UnixNano(), nil)
}

// CreateRetainedSnapshot creates a verified backup outside automatic retention.
// The caller holds the database lifecycle borrow; verification uses the copy.
func CreateRetainedSnapshot(ctx context.Context, db *sql.DB, target string, verify func(Executor) error) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := checkCanonicalDirectory(dir); err != nil {
		return err
	}
	return createBackupFile(ctx, db, target, time.Now().UnixNano(), verify)
}

func createBackupFile(ctx context.Context, db *sql.DB, target string, nonce int64, verify func(Executor) error) (resultErr error) {
	dir, name := filepath.Dir(target), filepath.Base(target)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.%d.tmp", name, nonce))

	// Reserve ownership before cleanup: never remove somebody else's file.
	file, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	identity, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	resource := preparedResource{path: tmp, info: identity}
	defer func() {
		if err := resource.remove(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary backup: %w", err))
		}
	}()
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		return err
	}
	// Validate and close the copy before publishing, including on Windows.
	copyDB, err := openSQLite(tmp, false, func(db *sql.DB) error {
		if err := checkSQLiteIntegrity(BindQueries(ctx, db)); err != nil {
			return err
		}
		if verify != nil {
			return verify(db)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := copyDB.Close(); err != nil {
		return err
	}
	file, err = os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := resource.checkIdentity(); err != nil {
		return err
	}
	if err := publishPreparedFile(tmp, target); err != nil {
		return err
	}
	// Do not delete an already published backup on a later durability error.
	return syncPreparedDirectory(dir)
}

// RemoveExpiredBackups 仅清理调用方提供的列表中早于 cutoff 的备份文件。
func RemoveExpiredBackups(dbPath string, files []BackupFile, cutoff time.Time) error {
	return RemoveExpiredBackupsContext(context.Background(), dbPath, files, cutoff)
}

func RemoveExpiredBackupsContext(ctx context.Context, dbPath string, files []BackupFile, cutoff time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := backupDirectory(dbPath)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
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
