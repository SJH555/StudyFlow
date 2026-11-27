package sessions

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/shared/studycalendar"
)

// ManualPlanConflict 表示补录尚未保存，需确认它会占用未结束计划的时段。
type ManualPlanConflict struct {
	Message string `json:"message"`
	Token   string `json:"token"`
}

func (e *ManualPlanConflict) Error() string { return e.Message }

// 结束时间为零表示没有可靠边界；首尾相接不算重叠。
func intervalsOverlap(start, end, otherStart, otherEnd time.Time) bool {
	return (end.IsZero() || otherStart.Before(end)) &&
		(otherEnd.IsZero() || start.Before(otherEnd))
}

func checkRecordOverlap(db *persistence.Queries, start, end time.Time, excludeID string) error {
	modifier, err := dayModifier(db)
	if err != nil {
		return err
	}
	rows, err := db.Query(`SELECT COALESCE(`+timerSubjectLabel(modifier)+`,'学习项目'),
		t.start_at,t.end_at,t.paused_at,t.planned_end_at
		FROM sessions t`+timerSubjectJoin(modifier)+` WHERE t.deleted=0
		AND CAST(t.id AS TEXT)<>?
		AND (unixepoch(t.end_at) IS NULL OR unixepoch(t.end_at) >= unixepoch(?)
			OR (substr(t.end_at,-1)<>'Z' AND substr(t.end_at,-6,1) NOT IN ('+','-')))
		ORDER BY t.id`, excludeID, start.Format(time.RFC3339))
	if err != nil {
		return err
	}
	defer rows.Close()
	loc := time.Local
	if modifier != "'localtime'" {
		loc = studycalendar.Location
	}
	for rows.Next() {
		var subject, rawStart string
		var actualEnd, paused, plannedEnd sql.NullString
		if err := rows.Scan(&subject, &rawStart, &actualEnd, &paused, &plannedEnd); err != nil {
			return err
		}
		otherStart, err := parseOverlapTime(rawStart, loc)
		if err != nil {
			return err
		}
		var otherEnd time.Time
		switch {
		case actualEnd.Valid:
			otherEnd, err = parseOverlapTime(actualEnd.String, loc)
		case plannedEnd.Valid:
			otherEnd, err = time.Parse(time.RFC3339Nano, plannedEnd.String)
		case paused.Valid:
			// 无固定边界的旧计时，暂停后的区间由恢复操作重新检查；
			// 未暂停则不能假定它会在未来某个时刻结束。
			otherEnd, err = parseOverlapTime(paused.String, loc)
		}
		if err != nil {
			return err
		}
		if !otherEnd.IsZero() && !otherEnd.After(otherStart) {
			continue
		}
		if intervalsOverlap(start, end, otherStart, otherEnd) {
			label := otherStart.In(loc).Format("2006-01-02 15:04:05")
			if !otherEnd.IsZero() {
				label += " – " + otherEnd.In(loc).Format("15:04:05")
			} else {
				label += " 起，无固定结束时间"
			}
			if !actualEnd.Valid {
				label += "（计时尚未结束）"
			}
			return fmt.Errorf("与“%s”的学习记录时间冲突：%s", subject, label)
		}
	}
	return rows.Err()
}

func parseOverlapTime(value string, loc *time.Location) (time.Time, error) {
	at, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return at, nil
	}
	// 早期五表库允许没有偏移量的本地时间；只用于比较，不改原记录。
	if local, localErr := time.ParseInLocation("2006-01-02T15:04:05", value, loc); localErr == nil {
		return local, nil
	}
	return time.Time{}, err
}

func manualPlanConflict(db *persistence.Queries, subjectID int64, start, end, now time.Time) (*ManualPlanConflict, error) {
	if !end.After(now) {
		return nil, nil
	}
	loc := studycalendar.Location
	first := start
	if now.After(first) {
		first = now
	}
	first = first.In(loc)
	// 从前一天开始，兼容可能延续到次日的旧跨日计划。
	day := time.Date(first.Year(), first.Month(), first.Day()-1, 0, 0, 0, 0, loc)
	for ; day.Before(end); day = day.AddDate(0, 0, 1) {
		windows, err := plans.EffectiveWindows(db, day)
		if err != nil {
			return nil, err
		}
		for _, window := range windows {
			if !window.End.After(now) || !intervalsOverlap(first, end, window.Start, window.End) {
				continue
			}
			overlapStart, overlapEnd := start, end
			if window.Start.After(overlapStart) {
				overlapStart = window.Start
			}
			if window.End.Before(overlapEnd) {
				overlapEnd = window.End
			}
			message := fmt.Sprintf("补录会占用“%s”计划的 %s–%s。保存后，预计计时时段与这条记录重叠时，不能启动计时。",
				window.Subject, overlapStart.In(loc).Format("2006-01-02 15:04"), overlapEnd.In(loc).Format("15:04"))
			token := sha256.Sum256([]byte(fmt.Sprintf("%d/%d/%d/%d/%d/%d/%s",
				subjectID, start.Unix(), end.Unix(), window.PlanID, window.Start.Unix(), window.End.Unix(), window.Subject)))
			return &ManualPlanConflict{Message: message, Token: fmt.Sprintf("%x", token)}, nil
		}
	}
	return nil, nil
}
