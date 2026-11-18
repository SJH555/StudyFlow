package sessions

import (
	"database/sql"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studytime"
	"time"
)

// Completion 表示一个已结束的计时学习时段。
type Completion struct {
	Name  string `json:"name"`
	Saved bool   `json:"saved"`
	EndAt string `json:"endAt"`
}

// FinishExpired 关闭所有已到期的计时器，即使应用在到期时刻处于休眠。
// 返回最晚的实际结束时间。
func FinishExpired(db *persistence.Queries, now time.Time) (*Completion, error) {
	var result *Completion
	var err error
	err = db.WithTx(func(tx *persistence.Queries) error {
		result, err = finishExpired(tx, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func finishExpired(db *persistence.Queries, now time.Time) (*Completion, error) {
	rows, err := db.Query(`SELECT t.id,s.name,t.start_at,t.planned_end_at,t.resumed_at,t.paused_at,t.duration_seconds,t.break_started_at,t.break_used_seconds,p.start_time,p.duration_minutes,p.weekdays
		FROM sessions t JOIN subjects s ON s.id=t.subject_id LEFT JOIN plans p ON p.id=t.plan_id
		WHERE t.end_at IS NULL AND t.deleted=0 AND t.plan_id IS NOT NULL ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type dueSession struct {
		id, stored, breakUsed                  int64
		name, started                          string
		planned, resumed, paused, breakStarted sql.NullString
		planStart, weekdays                    sql.NullString
		planDuration                           sql.NullInt64
	}
	var dueSessions []dueSession
	for rows.Next() {
		var session dueSession
		if err = rows.Scan(&session.id, &session.name, &session.started, &session.planned, &session.resumed, &session.paused, &session.stored, &session.breakStarted, &session.breakUsed, &session.planStart, &session.planDuration, &session.weekdays); err != nil {
			break
		}
		dueSessions = append(dueSessions, session)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	var latest *Completion
	for _, session := range dueSessions {
		var end time.Time
		if session.planned.Valid {
			end, err = time.Parse(time.RFC3339Nano, session.planned.String)
			if err != nil {
				return nil, err
			}
		} else if session.planStart.Valid && session.planDuration.Valid && session.weekdays.Valid {
			started, parseErr := time.Parse(time.RFC3339Nano, session.started)
			if parseErr != nil {
				return nil, parseErr
			}
			var found bool
			end, found = schedule.ScheduledEnd(started, session.planStart.String, int(session.planDuration.Int64), session.weekdays.String)
			if !found {
				continue
			}
		} else {
			continue
		}
		if now.Before(end) {
			continue
		}
		if err := validateTimerTimes(session.started, session.resumed, session.paused, session.breakStarted); err != nil {
			return nil, err
		}
		seconds := studytime.Elapsed(session.started, nullableTimestamp(session.resumed), session.paused.Valid, session.stored, end)
		used := session.breakUsed + studytime.BreakElapsedSeconds(nullableTimestamp(session.breakStarted), end)
		deleted := 0
		if seconds < 600 {
			deleted = 1
		}
		result, updateErr := db.Exec(`UPDATE sessions SET end_at=?,duration_seconds=?,break_used_seconds=?,deleted=?,paused_at=NULL,resumed_at=NULL,break_started_at=NULL,break_notified=0 WHERE id=? AND end_at IS NULL AND deleted=0`, end.Format(time.RFC3339), seconds, used, deleted, session.id)
		if updateErr != nil {
			return nil, updateErr
		}
		changed, updateErr := result.RowsAffected()
		if updateErr != nil {
			return nil, updateErr
		}
		if changed > 0 && (latest == nil || end.After(parseCompletionEnd(latest.EndAt))) {
			latest = &Completion{Name: session.name, Saved: deleted == 0, EndAt: end.Format(time.RFC3339)}
		}
	}
	return latest, nil
}

func parseCompletionEnd(raw string) time.Time {
	end, _ := time.Parse(time.RFC3339Nano, raw)
	return end
}
