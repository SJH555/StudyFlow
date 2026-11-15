package goals

import "time"

// Goal 表示带有本地自然日截止日期的阶段目标。
type Goal struct {
	ID          int64
	Title       string
	Deadline    string
	Note        string
	CompletedAt *time.Time
	AcceptedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
