package plans

import (
	"database/sql"
	"exam_notify/internal/modules/schedule"
	"time"
)

// StudyTarget 表示某科目一天的计划学习时长。
type StudyTarget struct {
	SubjectName    string
	PlannedSeconds int64
}

// StudyTargetsForDay 保留历史复盘过滤规则：仅统计启用且未删除的计划。
func StudyTargetsForDay(db *sql.DB, date time.Time) (map[int64]StudyTarget, error) {
	planned := map[int64]StudyTarget{}
	rows, err := db.Query(`SELECT p.subject_id,s.name,p.duration_minutes,p.break_minutes,p.weekdays FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.active=1 AND p.deleted=0`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var name, weekdays string
		var duration, rest int
		if err = rows.Scan(&id, &name, &duration, &rest, &weekdays); err != nil {
			break
		}
		if !schedule.ContainsWeekday(weekdays, int(date.Weekday())) {
			continue
		}
		item, exists := planned[id]
		if !exists {
			item.SubjectName = name
		}
		item.PlannedSeconds += int64(max(0, duration-rest)) * 60
		planned[id] = item
	}
	if rowErr := rows.Err(); err == nil {
		err = rowErr
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	return planned, nil
}
