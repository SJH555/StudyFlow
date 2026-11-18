package persistence

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
)

// snapshotDatabase 将一致性副本写入调用方预留的私有文件。
func snapshotDatabase(ctx context.Context, db *sql.DB, path string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	slashPath := filepath.ToSlash(path)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	dsn := "file://" + (&url.URL{Path: slashPath}).EscapedPath() + "?mode=rw"
	backupErr := conn.Raw(func(driverConn any) error {
		source, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite connection does not support online backup")
		}
		backup, err := source.NewBackup(dsn)
		if err != nil {
			return err
		}
		var stepErr error
		for {
			if stepErr = ctx.Err(); stepErr != nil {
				break
			}
			var more bool
			more, stepErr = backup.Step(128)
			if stepErr != nil || !more {
				break
			}
		}
		stepErr = errors.Join(stepErr, ctx.Err())
		// Finish 会完成已完成的备份，或回滚未完成的备份；出错时它还会关闭目标，而 Commit 不会。
		return errors.Join(stepErr, backup.Finish())
	})
	return errors.Join(backupErr, conn.Close())
}
