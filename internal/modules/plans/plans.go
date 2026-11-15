package plans

import (
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

// Create 校验并新增一条学习计划。
func Create(db *sql.DB, x Plan) (Plan, error) {
	if x.SubjectID == 0 || x.Duration < 1 || x.StartTime == "" {
		return x, errors.New("subject and positive duration are required")
	}
	if x.BreakMinutes < 0 || x.BreakMinutes >= x.Duration {
		return x, errors.New("休息时长必须小于计划时长")
	}
	x.Note = strings.TrimSpace(x.Note)
	if utf8.RuneCountInString(x.Note) > 100 {
		return x, errors.New("计划备注最多 100 字")
	}
	if x.Weekdays == "" {
		x.Weekdays = "1,2,3,4,5,6,0"
	}
	if x.Color == "" {
		x.Color = "#007AFF"
	}
	if e := checkConflict(db, x); e != nil {
		return x, e
	}
	return insertPlan(db, x)
}

// Update 校验并更新一条学习计划。
func Update(db *sql.DB, x Plan) (Plan, error) {
	if x.ID == 0 || x.SubjectID == 0 || x.Duration < 1 || x.StartTime == "" || x.Weekdays == "" || x.BreakMinutes < 0 || x.BreakMinutes >= x.Duration {
		return x, errors.New("invalid plan")
	}
	x.Note = strings.TrimSpace(x.Note)
	if utf8.RuneCountInString(x.Note) > 100 {
		return x, errors.New("计划备注最多 100 字")
	}
	if e := checkConflict(db, x); e != nil {
		return x, e
	}
	return x, updatePlan(db, x)
}
