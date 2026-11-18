package sessions

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"time"
)

// CountForSubject 统计某科目下未删除的学习记录数量。
func CountForSubject(db *persistence.Queries, subjectID any) (int, error) {
	var count int
	err := db.QueryRow(`SELECT count(*) FROM sessions WHERE subject_id=? AND deleted=0`, subjectID).Scan(&count)
	return count, err
}

// HasActiveRest 判断当前是否存在未结束的休息。
func HasActiveRest(db *persistence.Queries, now time.Time) (bool, error) {
	var startedRaw string
	var breakMinutes int
	var used int64
	err := db.QueryRow(`SELECT t.break_started_at,p.break_minutes,t.break_used_seconds+
  COALESCE((SELECT SUM(previous.break_used_seconds) FROM sessions previous WHERE previous.plan_id=t.plan_id AND date(previous.start_at,'localtime')=date(t.start_at,'localtime') AND previous.id<t.id),0)
  FROM sessions t JOIN plans p ON p.id=t.plan_id
  WHERE t.end_at IS NULL AND t.deleted=0 AND t.paused_at IS NOT NULL AND t.break_started_at IS NOT NULL AND p.break_minutes>0
  ORDER BY t.id DESC LIMIT 1`).Scan(&startedRaw, &breakMinutes, &used)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
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
	rows, err := db.Query(`SELECT subject_id,COALESCE(SUM(duration_seconds),0) FROM sessions WHERE deleted=0 AND end_at IS NOT NULL AND date(start_at,'localtime')=? GROUP BY subject_id`, day)
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
	err := db.QueryRow(`SELECT count(*) FROM sessions WHERE deleted=0 AND end_at IS NOT NULL AND date(start_at,'localtime')=?`, day).Scan(&count)
	return count > 0, err
}
