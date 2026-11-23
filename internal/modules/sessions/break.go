package sessions

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"time"

	"exam_notify/internal/modules/schedule"
)

// DueBreakReminder 返回需要触发的休息结束提醒（科目名）及是否尚未通知。
func DueBreakReminder(db *persistence.Queries, now time.Time) (string, bool, error) {
	var name string
	var due bool
	var err error
	err = db.WithTx(func(tx *persistence.Queries) error {
		name, due, err = dueBreakReminder(tx, now)
		return err
	})
	if err != nil {
		return "", false, err
	}
	return name, due, nil
}

func dueBreakReminder(db *persistence.Queries, now time.Time) (string, bool, error) {
	var id int64
	var subject, startedRaw, sessionStartRaw, planStart, weekdays string
	var plannedEnd sql.NullString
	var minutes, planDuration int
	var used int64
	join, modifier, err := timerPlanSources(db, false)
	if err != nil {
		return "", false, err
	}
	name := timerSubjectLabel(modifier)
	err = db.QueryRow(`SELECT t.id,`+name+`,t.break_started_at,p.break_minutes,t.break_used_seconds+COALESCE((SELECT SUM(previous.break_used_seconds) FROM sessions previous WHERE previous.plan_id=t.plan_id AND date(previous.start_at,`+modifier+`)=date(t.start_at,`+modifier+`) AND previous.id<t.id),0),t.planned_end_at,t.start_at,p.start_time,p.duration_minutes,p.weekdays
        FROM sessions t`+join+timerSubjectJoin(modifier)+`
        WHERE t.end_at IS NULL AND t.paused_at IS NOT NULL AND t.break_started_at IS NOT NULL
		AND t.break_notified=0 AND t.deleted=0 AND p.break_minutes>0 ORDER BY t.id DESC LIMIT 1`).Scan(&id, &subject, &startedRaw, &minutes, &used, &plannedEnd, &sessionStartRaw, &planStart, &planDuration, &weekdays)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !plannedEnd.Valid {
		sessionStart, parseErr := time.Parse(time.RFC3339Nano, sessionStartRaw)
		if parseErr != nil {
			return "", false, parseErr
		}
		if end, found := schedule.ScheduledEnd(sessionStart, planStart, planDuration, weekdays); found {
			plannedEnd = sql.NullString{String: end.Format(time.RFC3339), Valid: true}
		}
	}
	if plannedEnd.Valid {
		end, parseErr := time.Parse(time.RFC3339Nano, plannedEnd.String)
		if parseErr != nil {
			return "", false, parseErr
		}
		if !now.Before(end) {
			return "", false, nil
		}
	}
	started, err := time.Parse(time.RFC3339Nano, startedRaw)
	if err != nil {
		return "", false, err
	}
	remaining := int64(minutes)*60 - used
	if remaining > 0 && now.Before(started.Add(time.Duration(remaining)*time.Second)) {
		return "", false, nil
	}
	res, err := db.Exec(`UPDATE sessions SET break_notified=1 WHERE id=? AND break_notified=0 AND paused_at IS NOT NULL`, id)
	if err != nil {
		return "", false, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return "", false, err
	}
	return subject, changed > 0, nil
}
