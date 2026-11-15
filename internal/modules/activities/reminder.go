package activities

import (
	"exam_notify/internal/modules/settings"
	"strconv"
	"time"
)

// ReminderDue 判断活动提醒在 now 时刻是否应触发。
func ReminderDue(now time.Time, config settings.ActivityReminderConfig) bool {
	if config.Enabled != "1" {
		return false
	}
	interval, parseErr := strconv.Atoi(config.Interval)
	if parseErr != nil || interval < 1 || interval > 1440 {
		interval = 30
	}
	if config.StartTime == "" {
		config.StartTime = "09:00"
	}
	start, parseErr := time.Parse("15:04", config.StartTime)
	if parseErr != nil {
		return false
	}
	elapsed := now.Hour()*60 + now.Minute() - (start.Hour()*60 + start.Minute())
	return elapsed >= 0 && elapsed%interval == 0
}
