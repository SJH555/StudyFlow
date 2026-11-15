package app

import (
	"database/sql"
	"os"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/settings"
)

// CheckBackup 执行自动备份策略：按配置创建新备份并清理过期备份。
func CheckBackup(db *sql.DB, dbPath string, now time.Time) error {
	config, err := settings.ReadBackupConfig(db)
	if err != nil {
		return err
	}
	files, err := persistence.ListBackups(dbPath, config.Enabled)
	if os.IsNotExist(err) && !config.Enabled {
		return nil
	}
	if err != nil {
		return err
	}

	var latest time.Time
	for _, file := range files {
		if file.Timestamp.After(latest) {
			latest = file.Timestamp
		}
	}
	if config.Enabled && (latest.IsZero() || !now.Before(latest.Add(config.Interval))) {
		if err := persistence.CreateBackup(db, dbPath, now); err != nil {
			return err
		}
	}
	return persistence.RemoveExpiredBackups(dbPath, files, now.Add(-config.Retention))
}
