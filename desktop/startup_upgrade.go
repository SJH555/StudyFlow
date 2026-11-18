package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/storage"
	"exam_notify/internal/infrastructure/upgrade"
)

// startupUpgrade is also the gate for ALL business bindings and background work.
// Its mutex protects status only; never hold it while running SQL or callbacks.
type startupUpgrade struct {
	mu         sync.Mutex
	status     startupStatus
	manager    *upgrade.Manager
	database   *persistence.Database
	expected   upgrade.State
	tasks      *desktopTasks
	activate   func() error
	onReady    func()
	backup     func(context.Context, *persistence.Access) (string, error)
	createdNew bool
}

type startupStatus struct {
	Phase        string `json:"phase"`
	Kind         string `json:"kind"`
	Current      int64  `json:"current"`
	Target       int64  `json:"target"`
	DatabasePath string `json:"databasePath"`
	BackupPath   string `json:"backupPath"`
	Message      string `json:"message"`
	Detail       string `json:"detail"`
}

func prepareStartupUpgrade(paths startupDatabasePaths) (*App, *startupUpgrade) {
	control := &startupUpgrade{status: startupStatus{Phase: "checking", Target: upgrade.TargetVersion}}
	manager, err := upgrade.New()
	control.manager = manager
	var database *persistence.Database
	if err == nil {
		// Recovery must validate compatibility before retiring the previous location.
		recovered := func(path string) (*sql.DB, error) {
			db, err := persistence.OpenUnprepared(path, false)
			if err != nil {
				return nil, err
			}
			err = persistence.WithStartupAccess(context.Background(), db, func(a *persistence.Access) error {
				state, err := manager.Inspect(context.Background(), a)
				if err != nil {
					return err
				}
				if state.Fresh {
					return errors.New("recovered database is empty")
				}
				return nil
			})
			if err != nil {
				return nil, errors.Join(err, db.Close())
			}
			return db, nil
		}
		database, err = openStartupDatabaseUsing(paths, recovered, func(choice storage.StartupData) (*sql.DB, error) {
			control.createdNew = choice.Create
			return openSelectedForUpgrade(choice)
		})
	}
	control.database = database
	app := newApp(database)
	control.tasks = &app.tasks
	if err == nil {
		err = database.WithAccess(context.Background(), func(a *persistence.Access) error {
			state, err := manager.Inspect(context.Background(), a)
			control.expected = state
			if err == nil && state.Fresh && !state.Versioned && !control.createdNew {
				return errors.New("已存在的数据库为空，为避免覆盖数据，已停止启动")
			}
			return err
		})
		control.status.Current = control.expected.Version
		control.status.DatabasePath = database.Path()
	}
	if err != nil {
		control.fail(err)
		if database != nil {
			if closeErr := database.Close(); closeErr != nil {
				log.Print(closeErr)
			}
		}
		return app, control
	}
	control.backup = control.createBackup
	if control.expected.Fresh {
		err = database.WithAccess(context.Background(), func(a *persistence.Access) error { return manager.Apply(context.Background(), a, control.expected) })
		if err != nil {
			control.fail(err)
			return app, control
		}
	}
	if control.expected.Fresh || !control.expected.NeedsUpgrade() {
		if err := database.SetCandidateValidator(manager.ValidateReady); err != nil {
			control.fail(err)
		} else {
			control.status.Phase = "ready"
			control.status.Current = control.status.Target
		}
	} else {
		control.status.Phase = "confirmation"
		if control.expected.Untracked {
			control.status.Kind = "baseline"
		}
		if control.expected.Historical {
			control.status.Kind = "historical"
		}
	}
	return app, control
}

func openSelectedForUpgrade(choice storage.StartupData) (*sql.DB, error) {
	if choice.Create || choice.LegacySource != "" {
		if err := os.MkdirAll(filepath.Dir(choice.DatabasePath), 0700); err != nil {
			return nil, err
		}
	}
	if choice.LegacySource != "" {
		if err := requireStartupDatabaseFile(choice.LegacySource); err != nil {
			return nil, err
		}
		if err := persistence.SnapshotLegacyDatabase(context.Background(), choice.LegacySource, choice.DatabasePath); err != nil {
			return nil, err
		}
	}
	if !choice.Create {
		if err := requireStartupDatabaseFile(choice.DatabasePath); err != nil {
			return nil, fmt.Errorf("保存的数据目录缺少 study.db 或无法读取：%w", err)
		}
	}
	return persistence.OpenUnprepared(choice.DatabasePath, choice.Create)
}

func (c *startupUpgrade) snapshot() startupStatus { c.mu.Lock(); defer c.mu.Unlock(); return c.status }
func (c *startupUpgrade) isReady() bool           { return c == nil || c.snapshot().Phase == "ready" }
func (c *startupUpgrade) fail(err error) {
	log.Printf("database startup: %v", err)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Phase = "blocked"
	c.status.Kind = "failed"
	c.status.Detail = err.Error()
	c.status.Message = "数据库准备未完成，请退出并检查错误信息。原有数据和已生成的备份会保留。"
	if errors.Is(err, upgrade.ErrNewerDatabase) {
		c.status.Kind = "newer"
		c.status.Message = "此数据库需要更高版本的 StudyFlow，请更新应用后再打开。"
	}
	if errors.Is(err, upgrade.ErrUnknownSchema) || errors.Is(err, upgrade.ErrInvalidHistory) {
		c.status.Kind = "unrecognized"
		c.status.Message = "无法确认数据库结构或版本记录，请保留数据并联系维护者检查。"
	}
}

func (c *startupUpgrade) createBackup(ctx context.Context, a *persistence.Access) (string, error) {
	path := filepath.Join(filepath.Dir(c.snapshot().DatabasePath), "backups", fmt.Sprintf("upgrade-%d-to-%d-%s-%d.db", c.expected.Version, c.expected.Target, time.Now().Format("20060102-150405"), time.Now().UnixNano()))
	err := a.WithTransactionProvider(ctx, func(db *sql.DB, _ persistence.PinnedTransactionRunner) error {
		return persistence.CreateRetainedSnapshot(ctx, db, path, func(copy persistence.Executor) error {
			return c.manager.VerifySnapshot(ctx, copy, c.expected)
		})
	})
	return path, err
}

// StartupService is deliberately separate from APIService: no business methods
// are callable until initialization has finished, even from a stale frontend.
type StartupService struct{ control *startupUpgrade }

func (s *StartupService) Status() startupStatus { return s.control.snapshot() }
func (s *StartupService) Confirm() (result startupStatus) {
	c := s.control
	c.mu.Lock()
	if c.status.Phase != "confirmation" {
		state := c.status
		c.mu.Unlock()
		return state
	}
	c.status.Phase = "upgrading"
	c.mu.Unlock()
	ctx, done, ok := c.tasks.enter()
	if !ok {
		c.fail(errDesktopStopping)
		return c.snapshot()
	}
	defer done()
	// Capture panic at the UI boundary only after the common transaction cleanup.
	defer func() {
		if p := recover(); p != nil {
			c.fail(fmt.Errorf("数据库升级异常：%v", p))
			if err := c.database.Close(); err != nil {
				log.Print(err)
			}
			result = c.snapshot()
		}
	}()
	err := c.database.WithAccess(ctx, func(a *persistence.Access) error {
		current, err := c.manager.Inspect(ctx, a)
		if err != nil {
			return err
		}
		if current != c.expected {
			return errors.New("database changed since upgrade confirmation")
		}
		path, err := c.backup(ctx, a)
		if path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				c.mu.Lock()
				c.status.BackupPath = path
				c.mu.Unlock()
			}
		}
		if err != nil {
			return fmt.Errorf("升级前备份失败：%w", err)
		}
		return c.manager.Apply(ctx, a, c.expected)
	})
	if err == nil {
		err = c.database.SetCandidateValidator(c.manager.ValidateReady)
	}
	if err == nil && c.activate != nil {
		err = c.activate()
	}
	if err != nil {
		c.fail(err)
		if closeErr := c.database.Close(); closeErr != nil {
			log.Print(closeErr)
		}
		return c.snapshot()
	}
	c.mu.Lock()
	c.status.Phase = "ready"
	c.status.Current = c.status.Target
	c.mu.Unlock()
	if c.onReady != nil {
		c.onReady()
	}
	return c.snapshot()
}
