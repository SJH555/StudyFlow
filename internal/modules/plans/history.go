package plans

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"
)

const (
	Active  = "active"
	Stopped = "stopped"
	Deleted = "deleted"
)

const versionColumns = `p.id,p.plan_id,p.subject_id,s.name,
	COALESCE(NULLIF(p.color,''),s.color),p.color,p.start_time,p.duration_minutes,
	p.break_minutes,p.weekdays,p.note,p.status,s.status,p.effective_day`

// versionsByDaySQL 先选日期有效的最后一版，再过滤状态，避免停用时重新选中旧启用版本。
const versionsByDaySQL = `SELECT d.value,` + versionColumns + `
	FROM json_each(?) d CROSS JOIN (SELECT DISTINCT plan_id FROM plan_versions WHERE kind='recurring') identity
	JOIN plan_versions p ON p.id=(
		SELECT pv.id FROM plan_versions pv WHERE pv.kind='recurring' AND pv.plan_id=identity.plan_id AND pv.effective_day<=d.value
		ORDER BY pv.effective_day DESC,pv.id DESC LIMIT 1
	)
	JOIN subject_versions s ON s.id=(
		SELECT sv.id FROM subject_versions sv WHERE sv.subject_id=p.subject_id AND sv.effective_day<=d.value
		ORDER BY sv.effective_day DESC,sv.id DESC LIMIT 1
	) ORDER BY d.value,p.start_time,p.plan_id`

func versionsByDay(db *persistence.Queries, days []string) (map[string][]Plan, error) {
	encoded, err := json.Marshal(days)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(versionsByDaySQL, string(encoded))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]Plan, len(days))
	for rows.Next() {
		var day, subjectStatus string
		var p Plan
		err := rows.Scan(&day, &p.VersionID, &p.ID, &p.SubjectID, &p.Subject,
			&p.Color, &p.ColorOverride, &p.StartTime, &p.Duration, &p.BreakMinutes, &p.Weekdays,
			&p.Note, &p.Status, &subjectStatus, &p.EffectiveDay)
		if err != nil {
			return nil, err
		}
		p.Active = p.Status == Active && subjectStatus == Active
		out[day] = append(out[day], p)
	}
	return out, rows.Err()
}

// AsOf 返回某个自然日生效的循环模板，星期过滤由日历调用者处理。
func AsOf(db *persistence.Queries, day string) ([]Plan, error) {
	all, err := versionsByDay(db, []string{day})
	if err != nil {
		return nil, err
	}
	items := make([]Plan, 0)
	for _, p := range all[day] {
		if p.Active {
			items = append(items, p)
		}
	}
	return items, nil
}

type DailyHistory struct {
	AdoptionDay bool            `json:"adoptionDay,omitempty"`
	Kind        string          `json:"kind"` // complete、legacy 或 unknown
	Plans       []Plan          `json:"plans"`
	TargetsJSON json.RawMessage `json:"targets,omitempty"`
}

// DailyPlans 批量返回历史明细。已有旧复盘目标原样保留，缺失历史不以当前计划估算。
func DailyPlans(db *persistence.Queries, days []string, now time.Time) (map[string]DailyHistory, error) {
	if len(days) == 0 || len(days) > 62 {
		return nil, errors.New("请选择 1 到 62 个日期")
	}
	var first string
	var adopted sql.NullString
	err := db.QueryRow(`SELECT effective_day,adoption_day FROM plan_versions WHERE kind='history_boundary'`).Scan(&first, &adopted)
	if err != nil {
		return nil, err
	}
	items := make(map[string]DailyHistory, len(days))
	complete := make([]string, 0, len(days))
	weekdays := make(map[string]int, len(days))
	today := studycalendar.Day(now)
	for _, day := range days {
		date, err := time.Parse(time.DateOnly, day)
		if err != nil {
			return nil, err
		}
		if _, duplicate := weekdays[day]; duplicate {
			continue
		}
		weekdays[day] = int(date.Weekday())
		items[day] = DailyHistory{Kind: "unknown", Plans: []Plan{}}
		if day >= first || day >= today || day == adopted.String {
			complete = append(complete, day)
			items[day] = DailyHistory{Kind: "complete", Plans: []Plan{}, AdoptionDay: day == adopted.String}
			continue
		}
		var targets string
		err = db.QueryRow(`SELECT legacy_targets_json FROM plan_versions WHERE kind='legacy_target' AND effective_day=?`, day).Scan(&targets)
		if err == nil {
			if !json.Valid([]byte(targets)) {
				return nil, errors.New("旧复盘目标格式无效")
			}
			items[day] = DailyHistory{Kind: "legacy", Plans: []Plan{}, TargetsJSON: json.RawMessage(targets)}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if len(complete) == 0 {
		return items, nil
	}
	versions, err := versionsByDay(db, complete)
	if err != nil {
		return nil, err
	}
	for _, day := range complete {
		entry := items[day]
		for _, p := range versions[day] {
			if p.Active && schedule.ContainsWeekday(p.Weekdays, weekdays[day]) {
				entry.Plans = append(entry.Plans, p)
			}
		}
		items[day] = entry
	}
	return items, nil
}

// Management 返回编辑所用的最新版本和今日状态，今日排程仍独立按日期查询。
func Management(db *persistence.Queries, now time.Time) ([]Plan, error) {
	today, tomorrow := studycalendar.Day(now), studycalendar.Tomorrow(now)
	all, err := versionsByDay(db, []string{today, tomorrow})
	if err != nil {
		return nil, err
	}
	current := make(map[int64]Plan, len(all[today]))
	for _, p := range all[today] {
		current[p.ID] = p
	}
	items := make([]Plan, 0, len(all[tomorrow]))
	for _, p := range all[tomorrow] {
		old, exists := current[p.ID]
		if p.Status == Deleted {
			// 只保留今天仍存在的计划。当天新增又取消的计划没有已生效的版本。
			if exists && old.Status != Deleted {
				old.Status = Deleted
				old.ManagedActive = old.Active
				old.Pending = true
				old.PendingLabel = "明日起删除"
				old.PendingDay = tomorrow
				items = append(items, old)
			}
			continue
		}
		p.ManagedActive = !exists || old.Active
		p.PendingStatus = p.Status != old.Status && (exists || p.Status == Stopped)
		// 撤销尚未生效的停用会追加一条记录。排程内容相同时不再显示待变更。
		changed := old.VersionID != p.VersionID && (old.Status != p.Status ||
			old.SubjectID != p.SubjectID || old.storedColor() != p.storedColor() ||
			old.StartTime != p.StartTime || old.Duration != p.Duration ||
			old.BreakMinutes != p.BreakMinutes || old.Weekdays != p.Weekdays || old.Note != p.Note)
		p.Pending = !exists || old.Active != p.Active || changed
		switch {
		case p.ManagedActive && !p.Active:
			p.PendingLabel = "明日起停用"
		case !exists || !old.Active && p.Active:
			p.PendingLabel = "明日生效"
		case p.Pending:
			p.PendingLabel = "明日起变更"
		}
		if p.Pending {
			p.PendingDay = tomorrow
		}
		items = append(items, p)
	}
	return items, nil
}

// HistoryVersions 返回单条计划的版本，按生效日期及保存顺序倒排。
func HistoryVersions(db *persistence.Queries, id int64) ([]Plan, error) {
	rows, err := db.Query(`SELECT `+versionColumns+` FROM plan_versions p JOIN subject_versions s ON s.id=(
		SELECT sv.id FROM subject_versions sv WHERE sv.subject_id=p.subject_id AND sv.effective_day<=p.effective_day
		ORDER BY sv.effective_day DESC,sv.id DESC LIMIT 1
	) WHERE p.kind='recurring' AND p.plan_id=? ORDER BY p.effective_day DESC,p.id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Plan, 0)
	for rows.Next() {
		var p Plan
		var subjectStatus string
		if err := rows.Scan(&p.VersionID, &p.ID, &p.SubjectID, &p.Subject, &p.Color, &p.ColorOverride, &p.StartTime,
			&p.Duration, &p.BreakMinutes, &p.Weekdays, &p.Note, &p.Status, &subjectStatus, &p.EffectiveDay); err != nil {
			return nil, err
		}
		p.Active = p.Status == Active && subjectStatus == Active
		out = append(out, p)
	}
	return out, rows.Err()
}

// SaveVersion 以次日为生效日追加计划版本，计划 ID 与历史版本保持稳定。
func SaveVersion(db *persistence.Queries, p Plan, create bool, now time.Time) (Plan, error) {
	p.Note = strings.TrimSpace(p.Note)
	if p.SubjectID == 0 || p.Duration < 1 || p.Duration > 1440 || p.BreakMinutes < 0 ||
		p.BreakMinutes >= p.Duration || utf8.RuneCountInString(p.Note) > 100 {
		return p, errors.New("学习计划无效，请检查科目、时长、休息时间和备注")
	}
	day := studycalendar.Tomorrow(now)
	start, err := schedule.StartMinute(p.StartTime)
	if err != nil {
		return p, err
	}
	days, err := schedule.ParseWeekdays(p.Weekdays)
	if err != nil {
		return p, err
	}
	err = db.WithTx(func(tx *persistence.Queries) error {
		if create {
			p.Status = Active
		} else {
			old, err := versionForChange(tx, p.ID, day)
			if err != nil {
				return err
			}
			oldDays, oldDaysErr := schedule.ParseWeekdays(old.Weekdays)
			if oldDaysErr == nil && oldDays == days && old.SubjectID == p.SubjectID &&
				old.StartTime == p.StartTime && old.Duration == p.Duration &&
				old.BreakMinutes == p.BreakMinutes && old.storedColor() == p.storedColor() &&
				old.Note == p.Note {
				p = old
				p.Unchanged = true
				return nil
			}
			p.Status = old.Status
		}
		if err := validateVersionSubject(tx, p, day); err != nil {
			return err
		}
		if p.Status == Active {
			if err := checkVersionConflict(tx, p, day, start, days); err != nil {
				return err
			}
		}
		if create {
			if err := tx.QueryRow(`SELECT MAX(last_id)+1 FROM (
				SELECT MAX(plan_id) AS last_id FROM plan_versions WHERE kind='recurring'
				UNION ALL SELECT MAX(plan_id) FROM sessions
				UNION ALL SELECT 0
			)`).Scan(&p.ID); err != nil {
				return err
			}
		}
		return appendVersion(tx, &p, day)
	})
	return p, err
}

func versionForChange(db *persistence.Queries, id int64, day string) (Plan, error) {
	all, err := versionsByDay(db, []string{day})
	if err != nil {
		return Plan{}, err
	}
	for _, p := range all[day] {
		if p.ID == id && p.Status != Deleted {
			return p, nil
		}
	}
	return Plan{}, errors.New("学习计划不存在或已删除")
}

func appendVersion(db *persistence.Queries, p *Plan, day string) error {
	res, err := db.Exec(`INSERT INTO plan_versions(plan_id,subject_id,effective_day,start_time,
		duration_minutes,break_minutes,weekdays,color,note,status) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.SubjectID, day, p.StartTime, p.Duration, p.BreakMinutes, p.Weekdays, p.storedColor(), p.Note, p.Status)
	if err != nil {
		return err
	}
	p.VersionID, err = res.LastInsertId()
	p.EffectiveDay = day
	return err
}

func validateVersionSubject(db *persistence.Queries, p Plan, day string) error {
	var status string
	err := db.QueryRow(`SELECT status FROM subject_versions WHERE subject_id=? AND effective_day<=?
		ORDER BY effective_day DESC,id DESC LIMIT 1`, p.SubjectID, day).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("学习项目不存在")
	}
	if err != nil {
		return err
	}
	if p.Status == Active && status != Active {
		return errors.New("学习项目在计划生效日已停用或删除")
	}
	return nil
}

func checkVersionConflict(db *persistence.Queries, p Plan, day string, start int, days [7]bool) error {
	items, err := AsOf(db, day)
	if err != nil {
		return err
	}
	for _, existing := range items {
		if existing.ID == p.ID {
			continue
		}
		oldStart, err := schedule.StartMinute(existing.StartTime)
		if err != nil {
			return err
		}
		oldDays, err := schedule.ParseWeekdays(existing.Weekdays)
		if err != nil {
			return err
		}
		if wd, collision := schedule.Overlap(start, p.Duration, days, oldStart, existing.Duration, oldDays); collision {
			return fmt.Errorf("时间冲突：%s与“%s” %s 开始的计划重叠", schedule.WeekdayLabels[wd], existing.Subject, existing.StartTime)
		}
	}
	return nil
}

// ChangeStatus 追加停用、恢复或逻辑删除记录。
func ChangeStatus(db *persistence.Queries, id int64, status string, now time.Time) error {
	if status != Active && status != Stopped && status != Deleted {
		return errors.New("学习计划状态无效")
	}
	day := studycalendar.Tomorrow(now)
	return db.WithTx(func(tx *persistence.Queries) error {
		p, err := versionForChange(tx, id, day)
		if err != nil {
			return err
		}
		unchanged := p.Status == status
		p.Status = status
		if status == Active {
			if err := validateVersionSubject(tx, p, day); err != nil {
				return err
			}
			start, err := schedule.StartMinute(p.StartTime)
			if err != nil {
				return err
			}
			days, err := schedule.ParseWeekdays(p.Weekdays)
			if err != nil {
				return err
			}
			if err := checkVersionConflict(tx, p, day, start, days); err != nil {
				return err
			}
		}
		if unchanged {
			return nil
		}
		return appendVersion(tx, &p, day)
	})
}

// ForTimerStart 在事务内取得实际使用的版本及其计时边界。
func ForTimerStart(db *persistence.Queries, id int64, now time.Time) (Plan, time.Time, error) {
	now = now.In(studycalendar.Location)
	today := studycalendar.Day(now)
	yesterday := studycalendar.Day(now.AddDate(0, 0, -1))
	all, err := versionsByDay(db, []string{today, yesterday})
	if err != nil {
		return Plan{}, time.Time{}, err
	}
	for _, day := range []string{yesterday, today} {
		for _, p := range all[day] {
			if p.ID != id || !p.Active {
				continue
			}
			date, _ := time.ParseInLocation(time.DateOnly, day, now.Location())
			if !schedule.ContainsWeekday(p.Weekdays, int(date.Weekday())) {
				continue
			}
			minute, err := schedule.StartMinute(p.StartTime)
			if err != nil {
				return Plan{}, time.Time{}, err
			}
			end := date.Add(time.Duration(minute+p.Duration) * time.Minute)
			if now.Before(end) {
				return p, end, nil
			}
		}
	}
	return Plan{}, time.Time{}, errors.New("该计划今天未生效或学习时段已结束")
}

// ValidateSubjectSchedule 校验恢复项目后重新启用的计划与其他排程是否冲突。
func ValidateSubjectSchedule(db *persistence.Queries, subjectID int64, day string) error {
	items, err := AsOf(db, day)
	if err != nil {
		return err
	}
	for _, plan := range items {
		if plan.SubjectID != subjectID {
			continue
		}
		start, err := schedule.StartMinute(plan.StartTime)
		if err != nil {
			return err
		}
		days, err := schedule.ParseWeekdays(plan.Weekdays)
		if err != nil {
			return err
		}
		if err := checkVersionConflict(db, plan, day, start, days); err != nil {
			return err
		}
	}
	return nil
}
