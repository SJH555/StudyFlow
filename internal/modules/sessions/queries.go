package sessions

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"time"
)

// HasActiveRest 判断当前是否存在未结束的休息。
func HasActiveRest(db *persistence.Queries, now time.Time) (bool, error) {
	var startedRaw string
	var end sql.NullString
	var breakMinutes int
	var used int64
	join, modifier, err := timerPlanSources(db, false)
	if err != nil {
		return false, err
	}
	err = db.QueryRow(`SELECT t.break_started_at,p.break_minutes,t.break_used_seconds+
  COALESCE((SELECT SUM(previous.break_used_seconds) FROM sessions previous WHERE previous.plan_id=t.plan_id AND date(previous.start_at,`+modifier+`)=date(t.start_at,`+modifier+`) AND previous.id<t.id),0),t.planned_end_at
  FROM sessions t `+join+`
  WHERE t.end_at IS NULL AND t.deleted=0 AND t.paused_at IS NOT NULL AND t.break_started_at IS NOT NULL AND p.break_minutes>0
  ORDER BY t.id DESC LIMIT 1`).Scan(&startedRaw, &breakMinutes, &used, &end)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
		if end.Valid {
			boundary, err := time.Parse(time.RFC3339Nano, end.String)
			if err != nil || !now.Before(boundary) {
				return false, err
			}
		}
		started, parseErr := time.Parse(time.RFC3339Nano, startedRaw)
		if parseErr != nil {
			return false, parseErr
		}
		if remaining := int64(breakMinutes)*60 - used; remaining > 0 && now.Before(started.Add(time.Duration(remaining)*time.Second)) {
			return true, nil
		}
	}
	return false, nil
}

// StudySecondsBySubject 按科目统计指定日期的学习时长（秒）。
func StudySecondsBySubject(db *persistence.Queries, day string) (map[int64]int64, error) {
	actual := map[int64]int64{}
	modifier, err := dayModifier(db)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT subject_id,COALESCE(SUM(duration_seconds),0) FROM sessions WHERE deleted=0 AND end_at IS NOT NULL AND date(start_at,`+modifier+`)=? GROUP BY subject_id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, seconds int64
		if err = rows.Scan(&id, &seconds); err != nil {
			break
		}
		actual[id] = seconds
	}
	if rowErr := rows.Err(); err == nil {
		err = rowErr
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	return actual, nil
}

// HasStudyRecord 判断指定日期是否存在学习记录。
func HasStudyRecord(db *persistence.Queries, day string) (bool, error) {
	var count int
	modifier, err := dayModifier(db)
	if err != nil {
		return false, err
	}
	err = db.QueryRow(`SELECT count(*) FROM sessions WHERE deleted=0 AND end_at IS NOT NULL AND date(start_at,`+modifier+`)=?`, day).Scan(&count)
	return count > 0, err
}
