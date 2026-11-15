package app

import (
	"time"

	"exam_notify/internal/modules/review"
)

// ApplyScheduleChange 在执行计划或科目变更前冻结历史目标。
// 之后的变更失败不会回滚已保存的快照。
func ApplyScheduleChange(reviews *review.Service, now time.Time, change func()) error {
	if err := reviews.SnapshotPastDailyTargets(now); err != nil {
		return err
	}
	change()
	return nil
}
