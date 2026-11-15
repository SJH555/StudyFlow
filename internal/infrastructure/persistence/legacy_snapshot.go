package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SnapshotLegacyDatabase 将历史数据库（含已提交的 WAL 事务）复制到新文件；调用方负责校验生成的 schema。
func SnapshotLegacyDatabase(ctx context.Context, source, target string) (resultErr error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)
	stage, err := os.MkdirTemp(dir, ".studyflow-legacy-")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(stage); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove legacy snapshot staging directory: %w", err))
		}
	}()

	// 在线备份以 mode=rw 打开目标。先私有预留文件并关闭，再交给 SQLite，Windows 上同样如此。
	file, err := os.OpenFile(filepath.Join(stage, "snapshot.db"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	stagedPath := file.Name()
	if err := file.Close(); err != nil {
		return err
	}
	if err := snapshotLegacySource(ctx, source, stagedPath); err != nil {
		return err
	}

	file, err = os.OpenFile(stagedPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return fmt.Errorf("sync legacy database snapshot: %w", err)
	}
	if err := publishPreparedFile(stagedPath, target); err != nil {
		return fmt.Errorf("publish legacy database snapshot %s: %w", target, err)
	}
	return syncPreparedDirectory(dir)
}

func snapshotLegacySource(ctx context.Context, source, target string) error {
	path := filepath.ToSlash(source)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// 只读模式能正常读取 WAL，但关闭时无法对源做 checkpoint；不可变连接则可能忽略实时变更。
	dsn := "file://" + (&url.URL{Path: path}).EscapedPath() + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	return errors.Join(snapshotDatabase(ctx, db, target), db.Close())
}
