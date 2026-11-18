package settings

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"time"
)

const (
	autoBackupEnabledKey = "autoBackupEnabled"
	backupInterval       = 48 * time.Hour
	backupRetention      = 48 * time.Hour
)

// BackupConfig 表示自动备份配置。
type BackupConfig struct {
	Enabled   bool
	Interval  time.Duration
	Retention time.Duration
}

// ReadBackupConfig 读取自动备份配置。
func ReadBackupConfig(db *persistence.Queries) (BackupConfig, error) {
	value, err := readSetting(db, autoBackupEnabledKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return BackupConfig{}, err
	}
	return BackupConfig{Enabled: value == "1", Interval: backupInterval, Retention: backupRetention}, nil
}
