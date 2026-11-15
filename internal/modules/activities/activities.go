package activities

import (
	"database/sql"
	"time"
)

// Input 表示一条活动记录的输入：内容与发生时间。
type Input struct {
	Content    string `json:"content"`
	HappenedAt string `json:"happenedAt"`
}

// List 返回指定日期的活动记录，day 为空时使用今天。
func List(db *sql.DB, day string) ([]map[string]any, error) {
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	rows, e := db.Query(`SELECT id,happened_at,content FROM activity_logs WHERE substr(happened_at,1,10)=? ORDER BY happened_at DESC`, day)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var at, content string
		if rows.Scan(&id, &at, &content) == nil {
			out = append(out, map[string]any{"id": id, "happenedAt": at, "content": content})
		}
	}

	return out, nil
}

// Add 新增一条活动记录并返回其 ID。
func Add(db *sql.DB, in Input) (int64, error) {
	if in.HappenedAt == "" {
		in.HappenedAt = time.Now().Format(time.RFC3339)
	}
	res, e := db.Exec(`INSERT INTO activity_logs(happened_at,content) VALUES(?,?)`, in.HappenedAt, in.Content)
	if e != nil {
		return 0, e
	}
	id, _ := res.LastInsertId()

	return id, nil
}
