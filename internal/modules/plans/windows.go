package plans

import (
	"exam_notify/internal/infrastructure/persistence"

	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"
	"time"
)

// Window 表示一个计划时段的起止时间。
type Window struct {
	Start, End time.Time
	PlanID     int64
	Subject    string
}

// EffectiveWindows 返回指定日期当天所有有效计划的时段。
func EffectiveWindows(db *persistence.Queries, now time.Time) ([]Window, error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return nil, err
	}
	if history {
		now = now.In(studycalendar.Location)
		items, err := AsOf(db, studycalendar.Day(now))
		if err != nil {
			return nil, err
		}
		windows := make([]Window, 0)
		for _, p := range items {
			if !schedule.ContainsWeekday(p.Weekdays, int(now.Weekday())) {
				continue
			}
			clock, err := schedule.StartMinute(p.StartTime)
			if err != nil {
				return nil, err
			}
			start := time.Date(now.Year(), now.Month(), now.Day(), clock/60, clock%60, 0, 0, now.Location())
			windows = append(windows, Window{Start: start, End: start.Add(time.Duration(p.Duration) * time.Minute), PlanID: p.ID, Subject: p.Subject})
		}
		return windows, nil
	}
	rows, err := db.Query(`SELECT p.start_time,p.duration_minutes,p.weekdays,p.id,s.name FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.active=1 AND p.deleted=0 AND s.deleted=0 AND s.archived=0 ORDER BY p.start_time,p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	local := now.Local()
	windows := make([]Window, 0)
	for rows.Next() {
		var clockRaw, weekdays, subject string
		var id int64
		var minutes int
		if err := rows.Scan(&clockRaw, &minutes, &weekdays, &id, &subject); err != nil {
			return nil, err
		}
		if !schedule.ContainsWeekday(weekdays, int(local.Weekday())) {
			continue
		}
		clock, err := time.Parse("15:04", clockRaw)
		if err != nil {
			return nil, err
		}
		start := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, local.Location())
		windows = append(windows, Window{Start: start, End: start.Add(time.Duration(minutes) * time.Minute), PlanID: id, Subject: subject})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return windows, nil
}
