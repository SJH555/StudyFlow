package sessions

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/settings"
	"exam_notify/internal/shared/studytime"
)

// Session 表示一条学习记录或进行中的学习时段。
type Session struct {
	ID               int64   `json:"id"`
	PlanID           *int64  `json:"planId"`
	SubjectID        int64   `json:"subjectId"`
	Subject          string  `json:"subject"`
	Color            string  `json:"color"`
	StartAt          string  `json:"startAt"`
	EndAt            *string `json:"endAt"`
	PlannedEndAt     *string `json:"plannedEndAt"`
	PausedAt         *string `json:"pausedAt"`
	BreakStartedAt   *string `json:"breakStartedAt"`
	BreakUsedSeconds int64   `json:"breakUsedSeconds"`
	ResumedAt        *string `json:"resumedAt"`
	Duration         int64   `json:"duration"`
	Source           string  `json:"source"`
	Note             string  `json:"note"`
}

// ManualInput 表示手动记录一条学习时段的输入。
type ManualInput struct {
	SubjectID int64  `json:"subjectId"`
	StartAt   string `json:"startAt"`
	EndAt     string `json:"endAt"`
	Note      string `json:"note"`
}

// TimerInput 表示计时器操作的输入。
type TimerInput struct {
	Action    string `json:"action"`
	SubjectID int64  `json:"subjectId"`
	PlanID    int64  `json:"planId"`
	Note      string `json:"note"`
}

// List 返回指定日期的学习记录。
func List(db *sql.DB, day string) ([]Session, error) {
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	rows, e := db.Query(`SELECT t.id,t.plan_id,t.subject_id,s.name,s.color,t.start_at,t.end_at,t.planned_end_at,t.paused_at,t.resumed_at,t.break_started_at,t.break_used_seconds+COALESCE((SELECT SUM(previous.break_used_seconds) FROM sessions previous WHERE previous.plan_id=t.plan_id AND date(previous.start_at,'localtime')=date(t.start_at,'localtime') AND previous.id<t.id),0),t.duration_seconds,t.source,t.note FROM sessions t JOIN subjects s ON s.id=t.subject_id WHERE date(t.start_at,'localtime')=? AND t.deleted=0 ORDER BY julianday(t.start_at) DESC`, day)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		var end, plannedEnd sql.NullString
		var paused, resumed, breakStarted sql.NullString
		var planID sql.NullInt64
		if rows.Scan(&x.ID, &planID, &x.SubjectID, &x.Subject, &x.Color, &x.StartAt, &end, &plannedEnd, &paused, &resumed, &breakStarted, &x.BreakUsedSeconds, &x.Duration, &x.Source, &x.Note) == nil {
			if planID.Valid {
				x.PlanID = &planID.Int64
			}
			if end.Valid {
				x.EndAt = &end.String
			}
			if plannedEnd.Valid {
				x.PlannedEndAt = &plannedEnd.String
			}
			if paused.Valid {
				x.PausedAt = &paused.String
			}
			if breakStarted.Valid {
				x.BreakStartedAt = &breakStarted.String
			}
			if resumed.Valid {
				x.ResumedAt = &resumed.String
			}
			out = append(out, x)
		}
	}

	return out, nil
}

// AddManual 手动新增一条学习记录，并返回其 ID 与时长（秒）。
func AddManual(db *sql.DB, in ManualInput) (int64, int64, error) {
	start, e := time.Parse(time.RFC3339, in.StartAt)
	if e != nil {
		return 0, 0, e
	}
	end, e := time.Parse(time.RFC3339, in.EndAt)
	if e != nil || end.Before(start) {
		return 0, 0, errors.New("invalid session interval")
	}
	if !end.After(start) {
		return 0, 0, errors.New("结束时间必须晚于开始时间")
	}
	tx, e := db.Begin()
	if e != nil {
		return 0, 0, e
	}
	defer tx.Rollback()
	rows, e := tx.Query(`SELECT s.name,t.start_at,t.end_at,t.paused_at,t.planned_end_at FROM sessions t JOIN subjects s ON s.id=t.subject_id WHERE t.deleted=0`)
	if e != nil {
		return 0, 0, e
	}
	for rows.Next() {
		var subject, existingStartRaw string
		var existingEndRaw, existingPausedRaw, existingPlannedEndRaw sql.NullString
		if e = rows.Scan(&subject, &existingStartRaw, &existingEndRaw, &existingPausedRaw, &existingPlannedEndRaw); e != nil {
			break
		}
		existingStart, parseErr := time.Parse(time.RFC3339Nano, existingStartRaw)
		if parseErr != nil {
			e = parseErr
			break
		}
		// 保留运行中计划已安排时段的剩余时间。否则未来手动记录可能被接受，
		// 之后又与该计时时段重叠。
		existingEnd := time.Now()
		if existingEndRaw.Valid {
			existingEnd, parseErr = time.Parse(time.RFC3339Nano, existingEndRaw.String)
			if parseErr != nil {
				e = parseErr
				break
			}
		} else if existingPlannedEndRaw.Valid {
			existingEnd, parseErr = time.Parse(time.RFC3339Nano, existingPlannedEndRaw.String)
			if parseErr != nil {
				e = parseErr
				break
			}
		} else if existingPausedRaw.Valid {
			existingEnd, parseErr = time.Parse(time.RFC3339Nano, existingPausedRaw.String)
			if parseErr != nil {
				e = parseErr
				break
			}
		}
		if start.Before(existingEnd) && end.After(existingStart) {
			rows.Close()
			endLabel := existingEnd.Local().Format("15:04")
			if !existingEndRaw.Valid && !existingPausedRaw.Valid {
				endLabel += "（进行中）"
			} else if existingPausedRaw.Valid {
				endLabel += "（已暂停）"
			}
			return 0, 0, fmt.Errorf("与“%s”在 %s–%s 的学习记录时间冲突", subject, existingStart.Local().Format("15:04"), endLabel)
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return 0, 0, e
	}
	sec := int64(end.Sub(start).Seconds())
	res, e := tx.Exec(`INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source,note) VALUES(?,?,?,?,?,?)`, in.SubjectID, start.Format(time.RFC3339), end.Format(time.RFC3339), sec, "manual", in.Note)
	if e != nil {
		return 0, 0, e
	}
	if e = tx.Commit(); e != nil {
		return 0, 0, e
	}
	id, _ := res.LastInsertId()

	return id, sec, nil
}

func checkPlansRunning(db *sql.DB, now time.Time) error {
	paused, err := settings.PlansPausedToday(db, now)
	if err != nil {
		return err
	}
	if paused {
		return errors.New("今日计划已暂停，请先恢复")
	}
	return nil
}

// Start 结束上一个进行中的时段并启动一个新的计时时段。
func Start(db *sql.DB, in TimerInput, now time.Time) (int64, error) {
	if err := checkPlansRunning(db, now); err != nil {
		return 0, err
	}
	if in.SubjectID == 0 {
		return 0, errors.New("subject is required")
	}
	tx, e := db.Begin()
	if e != nil {
		return 0, e
	}
	var old, stored, oldBreakUsed int64
	var start string
	var resumed, paused, oldBreakStarted, oldPlannedEnd sql.NullString
	e = tx.QueryRow(`SELECT id,start_at,resumed_at,paused_at,duration_seconds,break_started_at,break_used_seconds,planned_end_at FROM sessions WHERE end_at IS NULL AND deleted=0 ORDER BY id DESC LIMIT 1`).Scan(&old, &start, &resumed, &paused, &stored, &oldBreakStarted, &oldBreakUsed, &oldPlannedEnd)
	if e == nil {
		oldEnd := now
		if oldPlannedEnd.Valid {
			if boundary, parseErr := time.Parse(time.RFC3339Nano, oldPlannedEnd.String); parseErr == nil && boundary.Before(oldEnd) {
				oldEnd = boundary
			}
		}
		seconds := studytime.Elapsed(start, nullableTimestamp(resumed), paused.Valid, stored, oldEnd)
		used := oldBreakUsed + studytime.BreakElapsedSeconds(nullableTimestamp(oldBreakStarted), oldEnd)
		if seconds < 600 {
			_, e = tx.Exec(`UPDATE sessions SET end_at=?,deleted=1,paused_at=NULL,resumed_at=NULL,break_started_at=NULL,break_used_seconds=? WHERE id=?`, oldEnd.Format(time.RFC3339), used, old)
		} else {
			_, e = tx.Exec(`UPDATE sessions SET end_at=?,duration_seconds=?,break_used_seconds=?,paused_at=NULL,resumed_at=NULL,break_started_at=NULL WHERE id=?`, oldEnd.Format(time.RFC3339), seconds, used, old)
		}
		if e != nil {
			_ = tx.Rollback()
			return 0, e
		}
	}
	plannedEnd, e := plans.EndForStart(tx, in.PlanID, now)
	if e != nil {
		_ = tx.Rollback()
		return 0, e
	}
	var plannedEndValue any
	if !plannedEnd.IsZero() {
		plannedEndValue = plannedEnd.Format(time.RFC3339)
	}
	res, e := tx.Exec(`INSERT INTO sessions(subject_id,plan_id,start_at,planned_end_at,source,note) VALUES(?,?,?,?,?,?)`, in.SubjectID, in.PlanID, now.Format(time.RFC3339), plannedEndValue, "timer", in.Note)
	if e != nil {
		_ = tx.Rollback()
		return 0, e
	}
	newID, _ := res.LastInsertId()
	if e = tx.Commit(); e != nil {
		return 0, e
	}

	return newID, nil
}

// ChangeTimer 执行计时器的暂停、恢复或停止操作。
func ChangeTimer(db *sql.DB, id, action string, now time.Time) (bool, error) {
	switch action {
	case "resume":
		if err := checkPlansRunning(db, now); err != nil {
			return false, err
		}
	case "pause", "pause-day", "stop":
	default:
		return false, errors.New("unknown action")
	}
	var start string
	var planID sql.NullInt64
	var resumed, paused, breakStarted, plannedEnd sql.NullString
	var stored, breakUsed int64
	e := db.QueryRow(`SELECT plan_id,start_at,resumed_at,paused_at,duration_seconds,break_started_at,break_used_seconds,planned_end_at FROM sessions WHERE id=? AND end_at IS NULL AND deleted=0`, id).Scan(&planID, &start, &resumed, &paused, &stored, &breakStarted, &breakUsed, &plannedEnd)
	if e != nil {
		return false, e
	}
	effectiveNow := now
	if plannedEnd.Valid {
		boundary, parseErr := time.Parse(time.RFC3339Nano, plannedEnd.String)
		if parseErr != nil {
			return false, parseErr
		}
		if !now.Before(boundary) {
			if action != "stop" {
				return false, errors.New("该学习时段已结束")
			}
			effectiveNow = boundary
		}
	}
	seconds := studytime.Elapsed(start, nullableTimestamp(resumed), paused.Valid, stored, effectiveNow)
	switch action {
	case "pause", "pause-day":
		if !paused.Valid {
			var breakMinutes int
			if action == "pause" {
				e = db.QueryRow(`SELECT COALESCE((SELECT p.break_minutes FROM sessions s JOIN plans p ON p.id=s.plan_id WHERE s.id=?),0)`, id).Scan(&breakMinutes)
				var usedToday int64
				if e == nil && planID.Valid && breakMinutes > 0 {
					e = db.QueryRow(`SELECT COALESCE(SUM(break_used_seconds),0) FROM sessions WHERE plan_id=? AND date(start_at,'localtime')=date(?,'localtime')`, planID.Int64, start).Scan(&usedToday)
				}
				if e == nil && breakMinutes > 0 && usedToday >= int64(breakMinutes)*60 {
					return false, errors.New("本次计划的休息时间已用完，请继续学习或结束本次学习")
				}
			}
			if e == nil {
				var breakStart any
				if breakMinutes > 0 {
					breakStart = now.Format(time.RFC3339)
				}
				_, e = db.Exec(`UPDATE sessions SET paused_at=?,resumed_at=NULL,duration_seconds=?,break_started_at=?,break_notified=0 WHERE id=?`, now.Format(time.RFC3339), seconds, breakStart, id)
			}
		} else if action == "pause-day" {
			used := breakUsed + studytime.BreakElapsedSeconds(nullableTimestamp(breakStarted), now)
			_, e = db.Exec(`UPDATE sessions SET break_started_at=NULL,break_used_seconds=?,break_notified=0 WHERE id=?`, used, id)
		}
	case "resume":
		if paused.Valid {
			used := breakUsed + studytime.BreakElapsedSeconds(nullableTimestamp(breakStarted), now)
			_, e = db.Exec(`UPDATE sessions SET paused_at=NULL,resumed_at=?,break_started_at=NULL,break_used_seconds=?,break_notified=0 WHERE id=?`, now.Format(time.RFC3339), used, id)
		}
	case "stop":
		used := breakUsed + studytime.BreakElapsedSeconds(nullableTimestamp(breakStarted), effectiveNow)
		if seconds < 600 {
			_, e = db.Exec(`UPDATE sessions SET end_at=?,deleted=1,paused_at=NULL,resumed_at=NULL,break_started_at=NULL,break_used_seconds=? WHERE id=?`, effectiveNow.Format(time.RFC3339), used, id)
		} else {
			_, e = db.Exec(`UPDATE sessions SET end_at=?,paused_at=NULL,resumed_at=NULL,break_started_at=NULL,duration_seconds=?,break_used_seconds=? WHERE id=?`, effectiveNow.Format(time.RFC3339), seconds, used, id)
		}
	}
	if e != nil {
		return false, e
	}

	return action == "stop" && seconds >= 600, nil
}

// Delete 软删除指定学习记录。
func Delete(db *sql.DB, id string) error {
	_, err := db.Exec(`UPDATE sessions SET deleted=1 WHERE id=? AND deleted=0 AND end_at IS NOT NULL`, id)
	return err
}

func nullableTimestamp(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
