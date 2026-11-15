package goals

import "time"

// Input 是创建或编辑阶段目标的请求。
type Input struct {
	ID       int64  `json:"id"` // 编辑时使用的目标标识。
	Title    string `json:"title"`
	Deadline string `json:"deadline"` // 本地自然日，格式为 YYYY-MM-DD。
	Note     string `json:"note"`
}

// Item 是返回给前端的目标数据，不暴露数据库存储结构。
type Item struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Deadline    string `json:"deadline"`
	Note        string `json:"note"`
	Completed   bool   `json:"completed"`
	Accepted    bool   `json:"accepted"`
	CompletedAt string `json:"completedAt,omitempty"`
	AcceptedAt  string `json:"acceptedAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func toItem(goal Goal) Item {
	item := Item{
		ID:        goal.ID,
		Title:     goal.Title,
		Deadline:  goal.Deadline,
		Note:      goal.Note,
		Completed: goal.CompletedAt != nil,
		Accepted:  goal.AcceptedAt != nil,
		CreatedAt: goal.CreatedAt.Format(time.RFC3339),
		UpdatedAt: goal.UpdatedAt.Format(time.RFC3339),
	}
	if goal.CompletedAt != nil {
		item.CompletedAt = goal.CompletedAt.Format(time.RFC3339)
	}
	if goal.AcceptedAt != nil {
		item.AcceptedAt = goal.AcceptedAt.Format(time.RFC3339)
	}
	return item
}
