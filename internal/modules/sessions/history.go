package sessions

import (
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/schedule"
)

func timerBreakMinutes(db *persistence.Queries, id string, minutes *int) error {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return err
	}
	join := "LEFT JOIN plans p ON p.id=t.plan_id"
	if history {
		join = "LEFT JOIN plan_versions p ON p.id=t.plan_version_id"
	}
	return db.QueryRow(`SELECT COALESCE(p.break_minutes,0) FROM sessions t `+join+` WHERE t.id=?`, id).Scan(minutes)
}

func breakUsedForDay(db *persistence.Queries, planID int64, start string, seconds *int64) error {
	modifier, err := dayModifier(db)
	if err != nil {
		return err
	}
	return db.QueryRow(`SELECT COALESCE(SUM(break_used_seconds),0) FROM sessions
		WHERE plan_id=? AND date(start_at,`+modifier+`)=date(?,`+modifier+`)`, planID, start).Scan(seconds)
}

func dayModifier(db *persistence.Queries) (string, error) {
	history, err := schedule.HasHistory(db)
	if history {
		// 计划生效规则按北京时间自然日，时间戳本身仍是绝对时间点。
		return "'+8 hours'", err
	}
	return "'localtime'", err
}

func timerPlanSources(db *persistence.Queries, left bool) (join, modifier string, err error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return "", "", err
	}
	mode := " JOIN "
	if left {
		mode = " LEFT JOIN "
	}
	if history {
		return mode + "plan_versions p ON p.id=t.plan_version_id", "'+8 hours'", nil
	}
	return mode + "plans p ON p.id=t.plan_id", "'localtime'", nil
}

func timerSubjectJoin(modifier string) string {
	if modifier == "'localtime'" {
		return " JOIN subjects s ON s.id=t.subject_id "
	}
	return " "
}

func timerSubjectLabel(modifier string) string {
	if modifier == "'localtime'" {
		return "s.name"
	}
	return `COALESCE((SELECT sv.name FROM subject_versions sv WHERE sv.subject_id=t.subject_id
		AND sv.effective_day<=date(t.start_at,'+8 hours')
		ORDER BY sv.effective_day DESC,sv.id DESC LIMIT 1),
		(SELECT sv.name FROM subject_versions sv WHERE sv.subject_id=t.subject_id
		ORDER BY sv.effective_day,sv.id LIMIT 1))`
}
