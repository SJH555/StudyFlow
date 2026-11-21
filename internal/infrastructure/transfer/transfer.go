// Package transfer 负责整库备份包，不维护数据库版本或升级 SQL。
package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/upgrade"
)

// Preview 描述已校验的导入副本，不暴露临时路径。
type Preview struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	ExportedAt    string `json:"exportedAt,omitempty"`
	Version       int64  `json:"version"`
	TargetVersion int64  `json:"targetVersion"`
	NeedsUpgrade  bool   `json:"needsUpgrade"`
	Legacy        bool   `json:"legacy"`
	Subjects      int64  `json:"subjects"`
	Sessions      int64  `json:"sessions"`
	TimezoneNote  string `json:"timezoneNote,omitempty"`
}

// Candidate 持有用户已选文件的隔离副本，原始文件不参与写入。
type Candidate struct {
	dir     string
	path    string
	state   upgrade.State
	manager *upgrade.Manager
	preview Preview
}

// CheckIdle 检查本机是否有尚未结束的计时，包括暂停和休息。
func CheckIdle(ctx context.Context, db *persistence.Database) error {
	return db.WithAccess(ctx, func(access *persistence.Access) error {
		return requireIdle(access.Queries(ctx), "本机")
	})
}

// Export 创建完整数据库快照，再在生命周期锁外打包。
func Export(ctx context.Context, db *persistence.Database, path string, now time.Time) error {
	manager, err := upgrade.New()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "studyflow-export-")
	if err != nil {
		return err
	}
	defer cleanup(dir)
	snapshot := filepath.Join(dir, "study.db")
	err = db.WithWrite(func(live *sql.DB, _ string) error {
		if err := requireIdle(persistence.BindQueries(ctx, live), "本机"); err != nil {
			return err
		}
		return persistence.CreateRetainedSnapshot(ctx, live, snapshot, func(copy persistence.Executor) error {
			return manager.ValidateReady(ctx, copy)
		})
	})
	if err != nil {
		return err
	}
	return writeArchive(ctx, path, snapshot, upgrade.TargetVersion, now)
}

// Prepare 复制、识别和预览备份。只有用户确认后才允许 Goose 修改这个副本。
func Prepare(ctx context.Context, path string, now time.Time) (*Candidate, error) {
	if err := checkSourceFile(path); err != nil {
		return nil, err
	}
	manager, err := upgrade.New()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "studyflow-import-")
	if err != nil {
		return nil, err
	}
	kept := false
	defer func() {
		if !kept {
			cleanup(dir)
		}
	}()

	c := &Candidate{dir: dir, path: filepath.Join(dir, "study.db"), manager: manager}
	c.preview.ID = filepath.Base(dir)
	c.preview.Filename = filepath.Base(path)
	var info manifest
	switch strings.ToLower(filepath.Ext(path)) {
	case Extension:
		info, err = readArchive(ctx, path, c.path)
	case ".db":
		err = persistence.SnapshotLegacyDatabase(ctx, path, c.path)
	default:
		return nil, errors.New("请选择 .studyflow 备份文件或旧版 study.db")
	}
	if err != nil {
		return nil, err
	}

	err = withCandidate(ctx, c.path, func(access *persistence.Access) error {
		c.state, err = manager.Inspect(ctx, access)
		if err != nil {
			return err
		}
		if c.state.Fresh {
			return errors.New("无法导入空数据库")
		}
		if info.Format != 0 && (!c.state.Versioned || c.state.Version != info.DatabaseVersion) {
			return errors.New("备份清单与数据库实际版本不一致")
		}
		q := access.Queries(ctx)
		if err := requireIdle(q, "备份文件中"); err != nil {
			return err
		}
		if err := q.QueryRow("SELECT count(*) FROM subjects").Scan(&c.preview.Subjects); err != nil {
			return err
		}
		return q.QueryRow("SELECT count(*) FROM sessions").Scan(&c.preview.Sessions)
	})
	if err != nil {
		if errors.Is(err, upgrade.ErrNewerDatabase) {
			return nil, errors.New("此数据库需要更新版本的 StudyFlow，请更新应用后再恢复")
		}
		return nil, fmt.Errorf("导入文件校验失败: %w", err)
	}

	c.preview.Version = c.state.Version
	c.preview.TargetVersion = c.state.Target
	c.preview.NeedsUpgrade = c.state.NeedsUpgrade()
	c.preview.Legacy = c.state.Untracked
	c.preview.ExportedAt = info.ExportedAt
	if info.ExportedAt == "" {
		c.preview.TimezoneNote = "旧版数据库没有时区信息。若来源设备时区不同，部分记录可能归入不同日期。"
	} else if exported, parseErr := time.Parse(time.RFC3339, info.ExportedAt); parseErr == nil {
		_, sourceOffset := exported.Zone()
		localZone, localOffset := exported.In(now.Location()).Zone()
		if sourceOffset != localOffset || info.TimeZone != localZone {
			c.preview.TimezoneNote = "备份时区与本机可能不同，部分记录可能归入不同的统计日期。"
		}
	}
	kept = true
	return c, nil
}

// Preview 返回给确认弹窗的数据。
func (c *Candidate) Preview() Preview { return c.preview }

// Close 清理隔离副本。原始文件不在临时目录中。
func (c *Candidate) Close() { cleanup(c.dir) }

// Restore 按现有 Goose 规则升级副本，完成校验后在原路径原子恢复。
func (c *Candidate) Restore(ctx context.Context, db *persistence.Database, committed func()) error {
	err := withCandidate(ctx, c.path, func(access *persistence.Access) error {
		if c.state.NeedsUpgrade() {
			// 用户已确认升级。原始备份文件保持不变，可作为升级前的副本。
			if err := c.manager.Apply(ctx, access, c.state); err != nil {
				return fmt.Errorf("升级导入副本失败: %w", err)
			}
		}
		if err := requireIdle(access.Queries(ctx), "备份文件中"); err != nil {
			return err
		}
		return access.WithTransactionProvider(ctx, func(copy *sql.DB, _ persistence.PinnedTransactionRunner) error {
			return c.manager.ValidateReady(ctx, copy)
		})
	})
	if err != nil {
		return err
	}

	// 回滚副本只在恢复期间存在。若极端错误导致回滚也失败，保留路径供恢复。
	rollbackDir, err := os.MkdirTemp(filepath.Dir(db.Path()), ".studyflow-rollback-")
	if err != nil {
		return fmt.Errorf("无法准备临时回滚目录: %w", err)
	}
	rollbackPath := filepath.Join(rollbackDir, "study.db")
	err = db.RestoreVerifiedSnapshot(ctx, c.path, rollbackPath, func(exec persistence.Executor) error {
		// 已提交后仍需完成校验，不能因应用退出的取消信号跳过。
		return c.manager.ValidateReady(context.WithoutCancel(ctx), exec)
	}, func(exec persistence.Executor) error {
		return requireIdle(persistence.BindQueries(ctx, exec), "本机")
	}, committed)
	if !errors.Is(err, persistence.ErrRestoreRecoveryUncertain) {
		cleanup(rollbackDir)
	}
	return err
}

func withCandidate(ctx context.Context, path string, use func(*persistence.Access) error) error {
	db, err := persistence.OpenUnprepared(path, false)
	if err != nil {
		return err
	}
	return errors.Join(persistence.WithStartupAccess(ctx, db, use), db.Close())
}

func requireIdle(query *persistence.Queries, where string) error {
	var hasDeletedColumn int
	if err := query.QueryRow(`SELECT count(*) FROM pragma_table_info('sessions') WHERE name='deleted'`).Scan(&hasDeletedColumn); err != nil {
		return err
	}
	condition := "end_at IS NULL"
	if hasDeletedColumn != 0 {
		condition += " AND deleted=0"
	}
	var active bool
	if err := query.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE " + condition + ")").Scan(&active); err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%s有未结束的学习计时（包括暂停或休息），请先结束计时再继续", where)
	}
	return nil
}

func cleanup(path string) {
	if err := os.RemoveAll(path); err != nil {
		log.Printf("清理临时迁移目录失败 %q: %v", path, err)
	}
}
