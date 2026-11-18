package plans

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
)

// List 返回所有未删除的学习计划。
func List(db *persistence.Queries) ([]Plan, error) {
	rows, e := db.Query(`SELECT p.id,p.subject_id,s.name,COALESCE(NULLIF(p.color,''),s.color),p.start_time,p.duration_minutes,p.break_minutes,p.weekdays,p.active,p.note FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.deleted=0 ORDER BY p.start_time,p.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var x Plan
		var active int
		if err := rows.Scan(&x.ID, &x.SubjectID, &x.Subject, &x.Color, &x.StartTime, &x.Duration, &x.BreakMinutes, &x.Weekdays, &active, &x.Note); err != nil {
			return nil, err
		}
		x.Active = active != 0
		out = append(out, x)

	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
func insertPlan(db *persistence.Queries, x Plan) (Plan, error) {
	res, e := db.Exec(`INSERT INTO plans(subject_id,start_time,duration_minutes,break_minutes,weekdays,active,color,note) VALUES(?,?,?,?,?,1,?,?)`, x.SubjectID, x.StartTime, x.Duration, x.BreakMinutes, x.Weekdays, x.Color, x.Note)
	if e != nil {
		return x, e
	}
	x.ID, e = res.LastInsertId()
	if e != nil {
		return x, e
	}
	return x, nil
}
func updatePlan(db *persistence.Queries, x Plan) error {
	result, err := db.Exec(`UPDATE plans SET subject_id=?,start_time=?,duration_minutes=?,break_minutes=?,weekdays=?,color=?,note=? WHERE id=? AND deleted=0`, x.SubjectID, x.StartTime, x.Duration, x.BreakMinutes, x.Weekdays, x.Color, x.Note, x.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("plan not found")
	}
	return nil
}

// Delete 软删除指定计划。
func Delete(db *persistence.Queries, id string) error {
	_, e := db.Exec(`UPDATE plans SET deleted=1,active=0 WHERE id=? AND deleted=0`, id)
	return e
}

func startingRows(db *persistence.Queries, clock string) (*sql.Rows, error) {
	// 已存在进行中计时的计划不再提示「现在可以开始」（提前开始等场景）。
	return db.Query(`SELECT p.id,s.name,p.start_time,p.weekdays FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.active=1 AND p.deleted=0 AND s.deleted=0 AND s.archived=0 AND p.start_time=? AND NOT EXISTS (SELECT 1 FROM sessions t WHERE t.plan_id=p.id AND t.end_at IS NULL AND t.deleted=0) ORDER BY p.id`, clock)
}

func currentRows(db *persistence.Queries) (*sql.Rows, error) {
	return db.Query(`SELECT p.id,s.name,p.start_time,p.duration_minutes,p.weekdays FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.active=1 AND p.deleted=0 AND s.deleted=0 AND s.archived=0 ORDER BY p.start_time,p.id`)
}

func readActiveTiming(exec BoundaryQuery, id int64, startTime *string, duration *int, weekdays *string) error {
	return exec.QueryRow(`SELECT start_time,duration_minutes,weekdays FROM plans WHERE id=? AND deleted=0 AND active=1`, id).Scan(startTime, duration, weekdays)
}

func conflictRows(db *persistence.Queries, id int64) (*sql.Rows, error) {
	return db.Query(`SELECT p.start_time,p.duration_minutes,p.weekdays,s.name FROM plans p JOIN subjects s ON s.id=p.subject_id WHERE p.deleted=0 AND p.active=1 AND p.id<>?`, id)
}

// CountForSubject 统计某科目下未删除的计划数量。
func CountForSubject(db *persistence.Queries, subjectID any) (int, error) {
	var count int
	err := db.QueryRow(`SELECT count(*) FROM plans WHERE subject_id=? AND deleted=0`, subjectID).Scan(&count)
	return count, err
}

// SetSubjectActive 批量设置某科目下所有计划的启用状态。
func SetSubjectActive(db *persistence.Queries, subjectID any, active bool) error {
	value := 0
	if active {
		value = 1
	}
	_, err := db.Exec(`UPDATE plans SET active=? WHERE subject_id=?`, value, subjectID)
	return err
}
