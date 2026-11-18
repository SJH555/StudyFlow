package settings

import (
	"exam_notify/internal/infrastructure/persistence"

	"errors"
)

// ResetTableInfo 描述一个可重置数据表及其记录数。
type ResetTableInfo struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

var resetTableLabels = map[string]string{"subjects": "学习项目", "plans": "学习计划", "sessions": "学习记录", "activity_logs": "活动记录", "settings": "应用设置", "daily_reviews": "昨日总结", "daily_review_items": "总结理由", "daily_plan_targets": "每日计划目标", "stage_goals": "阶段目标"}

// ListResetTables 返回所有可重置数据表及各自记录数。
func ListResetTables(db *persistence.Queries) ([]ResetTableInfo, error) {
	names, err := resettableNames(db)
	if err != nil {
		return nil, err
	}
	out := []ResetTableInfo{}
	for _, name := range names {
		count, err := resetTableCount(db, name)
		if err != nil {
			return nil, err
		}
		out = append(out, ResetTableInfo{Name: name, Label: resetTableLabels[name], Count: count})
	}
	return out, nil
}

// ResetTable 清空指定数据表（重置前会校验依赖关系）。
func resetTable(db *persistence.Queries, name string) error {
	if _, ok := resetTableLabels[name]; !ok {
		return errors.New("不支持重置此表")
	}
	if name == "subjects" {
		linked, err := subjectResetLinks(db)
		if err != nil {
			return err
		}
		if linked > 0 {
			return errors.New("请先重置学习记录和学习计划，再重置学习项目")
		}
	}
	return clearResetTable(db, name)
}

func ResetTable(db *persistence.Queries, name string) error {
	return db.WithTx(func(tx *persistence.Queries) error { return resetTable(tx, name) })
}
