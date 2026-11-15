package settings

import (
	"database/sql"
	"errors"
)

const activityReminderEnabledKey = "activityReminderEnabled"

// ActivityReminderConfig 表示活动提醒的相关配置。
type ActivityReminderConfig struct {
	Interval  string
	StartTime string
	Enabled   string
	SkipRest  string
}

// ReadActivityReminderConfig 读取活动提醒配置。
func ReadActivityReminderConfig(db *sql.DB) ActivityReminderConfig {
	// 每项设置都是可选的。当值缺失或无法读取时，保留调度器独立的回退逻辑。
	interval, _ := readSetting(db, "activityInterval")
	startTime, _ := readSetting(db, "activityStartTime")
	enabled, _ := readSetting(db, activityReminderEnabledKey)
	skipRest, _ := readSetting(db, "activitySkipRest")
	return ActivityReminderConfig{Interval: interval, StartTime: startTime, Enabled: enabled, SkipRest: skipRest}
}

// ActivityReminderEnabled 返回活动提醒是否开启。
func ActivityReminderEnabled(db *sql.DB) (bool, error) {
	value, err := readSetting(db, activityReminderEnabledKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return value == "1", err
}

// SetActivityReminderEnabled 设置活动提醒开关。
func SetActivityReminderEnabled(db *sql.DB, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	return writeSetting(db, activityReminderEnabledKey, value)
}
