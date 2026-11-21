package sessions

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

// 两个自然月最多 62 天，覆盖周/月联合查询，同时限制单次返回范围。
const maxSessionQueryDays = 62

// List 返回指定日期的学习记录，空日期使用今天，保留原单日查询行为。
func List(db *persistence.Queries, day string) ([]Session, error) {
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	grouped, err := listForDates(db, []string{day})
	if err != nil {
		return nil, err
	}
	return grouped[day], nil
}

// ListForDates 一次读取指定本地自然日的记录，未有记录的日期返回空数组。
func ListForDates(db *persistence.Queries, days []string) (map[string][]Session, error) {
	if len(days) > maxSessionQueryDays {
		return nil, fmt.Errorf("一次最多查询 %d 个日期", maxSessionQueryDays)
	}
	unique := make([]string, 0, len(days))
	seen := make(map[string]bool, len(days))
	for _, day := range days {
		parsed, err := time.Parse("2006-01-02", day)
		if err != nil || parsed.Format("2006-01-02") != day {
			return nil, fmt.Errorf("无效的查询日期：%s", day)
		}
		if !seen[day] {
			unique = append(unique, day)
			seen[day] = true
		}
	}
	return listForDates(db, unique)
}

func listForDates(db *persistence.Queries, days []string) (map[string][]Session, error) {
	out := make(map[string][]Session, len(days))
	args := make([]any, 0, len(days))
	for _, day := range days {
		out[day] = []Session{}
		args = append(args, day)
	}
	if len(days) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(days)), ",")
	// 本批日期只扫描一次；复用本地日期和记录，避免休息累计为每条记录回扫全库。
	// 删除记录必须在外层过滤，因为它们仍参与同一计划的当日休息累计。
	// NULL 计划无法匹配历史记录，只保留本条休息用量。
	query := `WITH selected AS MATERIALIZED (
        SELECT id,plan_id,subject_id,start_at,end_at,planned_end_at,paused_at,
            resumed_at,break_started_at,break_used_seconds,duration_seconds,source,note,deleted,
            date(start_at,'localtime') AS local_day
        FROM sessions WHERE date(start_at,'localtime') IN (` + placeholders + `)
    )
    SELECT t.local_day,
        t.id,t.plan_id,t.subject_id,s.name,s.color,t.start_at,t.end_at,
        t.planned_end_at,t.paused_at,t.resumed_at,t.break_started_at,
        t.break_used_seconds + CASE WHEN t.plan_id IS NULL THEN 0 ELSE COALESCE((
            SELECT SUM(previous.break_used_seconds) FROM selected previous
            WHERE previous.plan_id=t.plan_id
                AND previous.local_day=t.local_day
                AND previous.id<t.id
        ),0) END,
        t.duration_seconds,t.source,t.note
        FROM selected t JOIN subjects s ON s.id=t.subject_id
        WHERE t.deleted=0
        ORDER BY julianday(t.start_at) DESC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		var session Session
		var end, plannedEnd sql.NullString
		var paused, resumed, breakStarted sql.NullString
		var planID sql.NullInt64
		if err := rows.Scan(
			&day, &session.ID, &planID, &session.SubjectID, &session.Subject, &session.Color, &session.StartAt,
			&end, &plannedEnd, &paused, &resumed, &breakStarted,
			&session.BreakUsedSeconds, &session.Duration, &session.Source, &session.Note,
		); err != nil {
			return nil, err
		}
		if planID.Valid {
			session.PlanID = &planID.Int64
		}
		if end.Valid {
			session.EndAt = &end.String
		}
		if plannedEnd.Valid {
			session.PlannedEndAt = &plannedEnd.String
		}
		if paused.Valid {
			session.PausedAt = &paused.String
		}
		if breakStarted.Valid {
			session.BreakStartedAt = &breakStarted.String
		}
		if resumed.Valid {
			session.ResumedAt = &resumed.String
		}
		out[day] = append(out[day], session)

	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
