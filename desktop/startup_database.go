package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/storage"
)

type startupDatabasePaths struct {
	locationFile, defaultDir, installDir, legacyConfigDir string
}

func (p startupDatabasePaths) selectData() (storage.StartupData, error) {
	return storage.SelectStartupData(p.locationFile, p.defaultDir, p.installDir, p.legacyConfigDir)
}

func openStartupDatabase(paths startupDatabasePaths, openRecovered func(string) (*sql.DB, error)) (_ *persistence.Database, resultErr error) {
	var db *sql.DB
	var path string
	defer func() {
		if db != nil {
			if err := db.Close(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("close startup database %s: %w", path, err))
			}
		}
	}()

	recovered, err := storage.RecoverStartupLocation(paths.locationFile, func(selected string, unchanged bool, clear func() error) error {
		if unchanged {
			if err := paths.verifyUnchangedSelection(selected); err != nil {
				return err
			}
		}
		path = selected
		var err error
		db, err = openRecovered(path)
		if err != nil {
			return fmt.Errorf("validate recovered study database %s: %w", path, err)
		}
		if err := clear(); err != nil {
			if errors.Is(err, storage.ErrRetirementCleanup) {
				// 新库已经过验证；保留恢复记录供下次重试，清理失败不妨碍读取已提交的数据。
				log.Printf("数据目录已提交，旧文件清理尚未完成: %v", err)
				return nil
			}
			return fmt.Errorf("clear pending storage location: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if !recovered {
		choice, err := paths.selectData()
		if err != nil {
			return nil, err
		}
		path = choice.DatabasePath
		db, err = openSelectedStartupDatabase(choice)
		if err != nil {
			return nil, fmt.Errorf("open startup study database %s: %w", path, err)
		}
		if choice.PromoteDirectory != "" {
			if err := storage.PromoteStartupLocation(paths.locationFile, choice.PromoteDirectory); err != nil {
				return nil, fmt.Errorf("promote legacy storage location %s: %w", paths.locationFile, err)
			}
		}
	}

	database, err := persistence.AdoptDatabase(db, path)
	if err != nil {
		return nil, fmt.Errorf("adopt startup study database %s: %w", path, err)
	}
	db = nil // 所有权已转移给 Database。
	return database, nil
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

func requireStartupDatabaseFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("study database is not a regular file: %s", path)
	}
	return nil
}

func (p startupDatabasePaths) verifyUnchangedSelection(recoveredPath string) error {
	choice, err := p.selectData()
	if err != nil {
		return err
	}
	if choice.Create || choice.LegacySource != "" {
		return fmt.Errorf("recovered old study database does not match startup selection: %s", recoveredPath)
	}
	absolute, err := filepath.Abs(choice.DatabasePath)
	if err != nil {
		return err
	}
	selected, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("verify unchanged storage location: %w", err)
	}
	recovered, err := filepath.EvalSymlinks(recoveredPath)
	if err != nil {
		return fmt.Errorf("verify recovered old database: %w", err)
	}
	if selected != recovered {
		return fmt.Errorf("recovered old study database %s does not match startup selection %s", recoveredPath, selected)
	}
	return nil
}
