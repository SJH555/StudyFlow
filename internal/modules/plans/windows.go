package plans

import (
	"database/sql"
	"exam_notify/internal/modules/schedule"
	"time"
)

// Window 表示一个计划时段的起止时间。
type Window struct{ Start, End time.Time }

// EffectiveWindows 返回指定日期当天所有有效计划的时段。
func EffectiveWindows(db *sql.DB, now time.Time) ([]Window, error) {
	rows, err := db.Query(`SELECT p.start_time,p.duration_minutes,p.weekdays FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.active=1 AND p.deleted=0 AND s.deleted=0 AND s.archived=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	local := now.Local()
	windows := make([]Window, 0)
	for rows.Next() {
		var clockRaw, weekdays string
		var minutes int
		if err := rows.Scan(&clockRaw, &minutes, &weekdays); err != nil {
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
		windows = append(windows, Window{Start: start, End: start.Add(time.Duration(minutes) * time.Minute)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return windows, nil
}
