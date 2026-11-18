package settings

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
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
func ReadActivityReminderConfig(db *persistence.Queries) (ActivityReminderConfig, error) {
	var config ActivityReminderConfig
	for _, item := range []struct {
		key   string
		value *string
	}{
		{"activityInterval", &config.Interval}, {"activityStartTime", &config.StartTime},
		{activityReminderEnabledKey, &config.Enabled}, {"activitySkipRest", &config.SkipRest},
	} {
		value, err := readSetting(db, item.key)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ActivityReminderConfig{}, err
		}
		*item.value = value
	}
	return config, nil
}

// ActivityReminderEnabled 返回活动提醒是否开启。
func ActivityReminderEnabled(db *persistence.Queries) (bool, error) {
	value, err := readSetting(db, activityReminderEnabledKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return value == "1", err
}

// SetActivityReminderEnabled 设置活动提醒开关。
func SetActivityReminderEnabled(db *persistence.Queries, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	return writeSetting(db, activityReminderEnabledKey, value)
}
