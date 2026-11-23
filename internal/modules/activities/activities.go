package activities

import (
	"exam_notify/internal/infrastructure/persistence"

	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"
	"time"
)

// Input 表示一条活动记录的输入：内容与发生时间。
type Input struct {
	Content    string `json:"content"`
	HappenedAt string `json:"happenedAt"`
}

// List 返回指定日期的活动记录，day 为空时使用今天。
func List(db *persistence.Queries, day string) ([]map[string]any, error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return nil, err
	}
	dayExpr := "substr(happened_at,1,10)"
	if history {
		dayExpr = "date(happened_at,'+8 hours')"
	}
	if day == "" {
		day = studycalendar.Day(time.Now())
	}
	rows, e := db.Query(`SELECT id,happened_at,content FROM activity_logs WHERE `+dayExpr+`=? ORDER BY happened_at DESC`, day)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var at, content string
		if err := rows.Scan(&id, &at, &content); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "happenedAt": at, "content": content})

	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Add 新增一条活动记录并返回其 ID。
func add(db *persistence.Queries, in Input) (int64, error) {
	if in.HappenedAt == "" {
		in.HappenedAt = time.Now().Format(time.RFC3339)
	}
	res, e := db.Exec(`INSERT INTO activity_logs(happened_at,content) VALUES(?,?)`, in.HappenedAt, in.Content)
	if e != nil {
		return 0, e
	}
	id, e := res.LastInsertId()
	if e != nil {
		return 0, e
	}

	return id, nil
}

func Add(db *persistence.Queries, in Input) (int64, error) {
	var result int64
	err := db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = add(tx, in)
		return err
	})
	if err != nil {
		return 0, err
	}
	return result, nil
}
