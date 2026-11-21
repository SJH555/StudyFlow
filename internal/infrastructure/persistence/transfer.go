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

	"modernc.org/sqlite"
)

// ErrRestoreRecoveryUncertain 表示恢复失败且无法确认已回滚，须保留临时原库。
var ErrRestoreRecoveryUncertain = errors.New("无法确认恢复结果")

// WriteNewArchive 完整写入并刷新同目录的临时文件后发布，已有文件不会被覆盖。
func WriteNewArchive(ctx context.Context, path string, write func(*os.File) error) error {
	dir := filepath.Dir(path)
	output, err := os.CreateTemp(dir, ".studyflow-export-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	writeErr := write(output)
	if writeErr == nil {
		writeErr = output.Sync()
	}
	if err := errors.Join(writeErr, output.Close(), ctx.Err()); err != nil {
		return err
	}
	if err := publishPreparedFile(output.Name(), path); err != nil {
		return fmt.Errorf("无法保存备份文件（请选择尚不存在的文件名）: %w", err)
	}
	return syncPreparedDirectory(dir)
}

// RestoreVerifiedSnapshot 在独占生命周期借用中恢复整库，SQLite 在线恢复负责原子提交。
// 前置校验及临时回滚副本均在写入前完成；发布后再允许回调停止旧业务。
func (d *Database) RestoreVerifiedSnapshot(
	ctx context.Context, source, rollback string,
	verify func(Executor) error, preflight func(Executor) error, committed func(),
) error {
	return d.WithWrite(func(db *sql.DB, _ string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := preflight(db); err != nil {
			return err
		}
		if err := CreateRetainedSnapshot(ctx, db, rollback, verify); err != nil {
			return fmt.Errorf("无法生成临时回滚副本: %w", err)
		}

		changed, restoreErr := restoreDatabasePages(ctx, db, source)
		if !changed {
			return fmt.Errorf("恢复失败，原库保留: %w", restoreErr)
		}
		if err := errors.Join(restoreErr, verify(db)); err != nil {
			// 全部页面已切换，不能因取消跳过回滚。
			recovered, recoveryErr := restoreDatabasePages(context.WithoutCancel(ctx), db, rollback)
			if recoveryErr == nil && recovered {
				recoveryErr = verify(db)
			}
			if recoveryErr != nil || !recovered {
				d.pauseErr = fmt.Errorf("%w，已暂停数据库。请保留回滚副本 %s: %w", ErrRestoreRecoveryUncertain, rollback, errors.Join(err, recoveryErr))
				return d.pauseErr
			}
			return fmt.Errorf("恢复后校验失败，已恢复原库: %w", err)
		}
		committed()
		return nil
	})
}

// restoreDatabasePages 的提交点是 Step 返回 SQLITE_DONE。此前 Finish 会撤销未完成的复制。
func restoreDatabasePages(ctx context.Context, db *sql.DB, source string) (committed bool, resultErr error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, conn.Close()) }()
	absPath, err := filepath.Abs(source)
	if err != nil {
		return false, err
	}
	slashPath := filepath.ToSlash(absPath)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	dsn := "file://" + (&url.URL{Path: slashPath}).EscapedPath() + "?mode=ro"
	resultErr = conn.Raw(func(raw any) (err error) {
		restorer, ok := raw.(interface {
			NewRestore(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite 连接不支持在线恢复")
		}
		restore, err := restorer.NewRestore(dsn)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, restore.Finish()) }()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			// 每批 128 页，长操作可以在批次之间响应退出取消。
			more, err := restore.Step(128)
			if err != nil {
				return err
			}
			if !more {
				committed = true
				return nil
			}
		}
	})
	return committed, resultErr
}
