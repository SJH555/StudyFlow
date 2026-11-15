package app

import (
	"database/sql"
	"time"

	"exam_notify/internal/modules/activities"
	"exam_notify/internal/modules/settings"
)

// ActivityReminder 判断活动提醒现在是否应触发。它读取用户的活动提醒设置，
// 并在跳过休息时段时，检查用户当前是否正在休息。
func (s *Scheduler) ActivityReminder(now time.Time) (bool, error) {
	var config settings.ActivityReminderConfig
	err := s.db.WithRead(func(db *sql.DB, _ string) error {
		config = settings.ReadActivityReminderConfig(db)
		return nil
	})
	if err != nil {
		return false, err
	}
	if !activities.ReminderDue(now, config) {
		return false, nil
	}
	if config.SkipRest != "1" {
		return true, nil
	}
	var resting bool
	err = s.db.WithRead(func(db *sql.DB, _ string) error {
		var err error
		resting, err = activities.Resting(db, now)
		return err
	})
	if err != nil {
		return false, err
	}
	return !resting, nil
}
