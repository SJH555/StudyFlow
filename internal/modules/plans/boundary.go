package plans

import (
	"database/sql"
	"errors"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/modules/settings"
	"time"
)

// ReminderItem 表示计划提醒所需的简要信息。
type ReminderItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	StartTime string `json:"startTime"`
}

// StartingAt 返回在 now 时刻开始且当天未暂停的计划。
func StartingAt(db *sql.DB, now time.Time) (*ReminderItem, error) {
	if paused, err := settings.PlansPausedToday(db, now); err != nil || paused {
		return nil, err
	}
	rows, err := startingRows(db, now.Format("15:04"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ReminderItem
		var weekdays string
		if err := rows.Scan(&item.ID, &item.Name, &item.StartTime, &weekdays); err != nil {
			return nil, err
		}
		if schedule.ContainsWeekday(weekdays, int(now.Weekday())) {
			return &item, nil
		}
	}
	return nil, rows.Err()
}

// CurrentAt 返回在 now 时刻正在进行且当天未暂停的计划。
func CurrentAt(db *sql.DB, now time.Time) (*ReminderItem, error) {
	if paused, err := settings.PlansPausedToday(db, now); err != nil || paused {
		return nil, err
	}
	rows, err := currentRows(db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ReminderItem
		var duration int
		var weekdays string
		if err := rows.Scan(&item.ID, &item.Name, &item.StartTime, &duration, &weekdays); err != nil {
			return nil, err
		}
		if _, ok := schedule.ScheduledEnd(now, item.StartTime, duration, weekdays); ok {
			return &item, nil
		}
	}
	return nil, rows.Err()
}

// BoundaryQuery 是计划边界查询所需的读取能力。
// 在会话事务中，调用方会把该事务作为执行器传入。
type BoundaryQuery interface {
	QueryRow(query string, args ...any) *sql.Row
}

// EndForStart 返回指定计划在 now 时刻开始计时的时段结束时间。
// 提前开始（now 早于时段）时锚定到当天该时段的结束，使计时仍按计划时段自动结束。
func EndForStart(exec BoundaryQuery, planID int64, now time.Time) (time.Time, error) {
	if planID == 0 {
		return time.Time{}, nil
	}
	var startTime, weekdays string
	var duration int
	err := readActiveTiming(exec, planID, &startTime, &duration, &weekdays)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	end, _ := schedule.BoundaryEnd(now, startTime, duration, weekdays)
	return end, nil
}
