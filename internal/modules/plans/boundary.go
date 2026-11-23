package plans

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/modules/settings"
	"exam_notify/internal/shared/studycalendar"
	"fmt"
	"time"
)

// ReminderItem 表示计划提醒所需的简要信息。
type ReminderItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	StartTime string `json:"startTime"`
}

// StartingAt 返回在 now 时刻开始且当天未暂停的计划。
func StartingAt(db *persistence.Queries, now time.Time) (*ReminderItem, error) {
	if paused, err := settings.PlansPausedToday(db, now); err != nil || paused {
		return nil, err
	}
	history, err := schedule.HasHistory(db)
	if err != nil {
		return nil, err
	}
	if history {
		items, err := AsOf(db, studycalendar.Day(now))
		if err != nil {
			return nil, err
		}
		now = now.In(studycalendar.Location)
		for _, p := range items {
			if p.StartTime != now.Format("15:04") || !schedule.ContainsWeekday(p.Weekdays, int(now.Weekday())) {
				continue
			}
			var running bool
			if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sessions WHERE plan_id=? AND end_at IS NULL AND deleted=0)`, p.ID).Scan(&running); err != nil {
				return nil, err
			}
			if !running {
				return &ReminderItem{ID: p.ID, Name: p.Subject, StartTime: p.StartTime}, nil
			}
		}
		return nil, nil
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
func CurrentAt(db *persistence.Queries, now time.Time) (*ReminderItem, error) {
	if paused, err := settings.PlansPausedToday(db, now); err != nil || paused {
		return nil, err
	}
	history, err := schedule.HasHistory(db)
	if err != nil {
		return nil, err
	}
	if history {
		now = now.In(studycalendar.Location)
		for _, day := range []time.Time{now, now.AddDate(0, 0, -1)} {
			items, err := AsOf(db, studycalendar.Day(day))
			if err != nil {
				return nil, err
			}
			for _, p := range items {
				if !schedule.ContainsWeekday(p.Weekdays, int(day.Weekday())) {
					continue
				}
				clock, err := schedule.StartMinute(p.StartTime)
				if err != nil {
					return nil, err
				}
				start := time.Date(day.Year(), day.Month(), day.Day(), clock/60, clock%60, 0, 0, day.Location())
				if !now.Before(start) && now.Before(start.Add(time.Duration(p.Duration)*time.Minute)) {
					return &ReminderItem{ID: p.ID, Name: p.Subject, StartTime: p.StartTime}, nil
				}
			}
		}
		return nil, nil
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
	QueryRow(query string, args ...any) persistence.Row
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
	if _, err := schedule.StartMinute(startTime); err != nil {
		return time.Time{}, err
	}
	if _, err := schedule.ParseWeekdays(weekdays); err != nil {
		return time.Time{}, err
	}
	if duration < 1 || duration > 1440 {
		return time.Time{}, fmt.Errorf("计划时长必须在 1 到 1440 分钟之间")
	}
	end, _ := schedule.BoundaryEnd(now, startTime, duration, weekdays)
	return end, nil
}
