package main

import (
	"context"
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/storage"
	"fmt"
	"os"
	"path/filepath"
)

// These pre-Goose constructors build legacy fixtures and exercise historical
// startup tests. Production must use prepareStartupUpgrade, which checks the
// database before consent, backup, and schema changes. Keep these out of builds.
// NewApp 在指定路径上初始化数据库并创建应用实例。
func NewApp(path string) (*App, error) {
	database, err := persistence.NewDatabase(path)
	if err != nil {
		return nil, err
	}
	return newApp(database), nil
}

func openStartupDatabase(paths startupDatabasePaths, openRecovered func(string) (*sql.DB, error)) (*persistence.Database, error) {
	return openStartupDatabaseUsing(paths, openRecovered, openSelectedStartupDatabase)
}

func openSelectedStartupDatabase(choice storage.StartupData) (*sql.DB, error) {
	if choice.Create || choice.LegacySource != "" {
		if err := os.MkdirAll(filepath.Dir(choice.DatabasePath), 0700); err != nil {
			return nil, err
		}
	}
	if choice.LegacySource != "" {
		if err := requireStartupDatabaseFile(choice.LegacySource); err != nil {
			return nil, fmt.Errorf("检查原有学习数据库: %w", err)
		}
		if err := persistence.SnapshotLegacyDatabase(context.Background(), choice.LegacySource, choice.DatabasePath); err != nil {
			return nil, fmt.Errorf("迁移原有学习数据失败: %w", err)
		}
	}
	if choice.Create {
		return persistence.Open(choice.DatabasePath)
	}
	if err := requireStartupDatabaseFile(choice.DatabasePath); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("保存的数据目录缺少 study.db: %w", err)
	} else if err != nil {
		return nil, err
	}
	return persistence.OpenForStartup(choice.DatabasePath)
}
