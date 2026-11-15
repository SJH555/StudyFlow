package app

import (
	"database/sql"
	"fmt"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/review"
	"exam_notify/internal/modules/sessions"
)

// Scheduler 负责后台提醒与维护检查。它是一个普通对象：
// 桌面宿主在每次 tick 时驱动它，并自行持有并发控制与所有 UI 副作用。
type Scheduler struct {
	db *persistence.Database
}

// NewScheduler 以给定的数据库创建 Scheduler。
func NewScheduler(db *persistence.Database) *Scheduler {
	return &Scheduler{db: db}
}

// BackupCheck 执行自动备份策略及其数据库操作。
// 由桌面宿主决定何时调用。
func (s *Scheduler) BackupCheck(now time.Time) error {
	return s.db.WithWrite(func(db *sql.DB, path string) error {
		return CheckBackup(db, path, now)
	})
}

// BreakReminder 报告某个暂停时段的休息是否已用完，返回需要展示的科目以及该提醒是否尚未触发。
// 底层查询会把该时段标记为已通知，因此在写锁下执行。
func (s *Scheduler) BreakReminder(now time.Time) (string, bool, error) {
	var subject string
	var due bool
	err := s.db.WithWrite(func(db *sql.DB, _ string) error {
		var err error
		subject, due, err = sessions.DueBreakReminder(db, now)
		return err
	})
	return subject, due, err
}

// PendingReview 返回最早的一条未完成每日总结，没有到期项时返回 nil。
// 查询只读，因此在读锁下执行。
func (s *Scheduler) PendingReview(now time.Time) (*review.DailyReview, error) {
	var pending *review.DailyReview
	err := s.db.WithRead(func(db *sql.DB, _ string) error {
		var err error
		pending, err = review.New(db).Pending(now)
		return err
	})
	return pending, err
}

// DailySnapshot 在每日总结开启时保存昨天的计划目标。
// 桌面宿主在午夜调用一次。
func (s *Scheduler) DailySnapshot(now time.Time) error {
	return s.db.WithRead(func(db *sql.DB, _ string) error {
		reviews := review.New(db)
		enabled, _, _, _, err := reviews.ReviewConfig()
		if err != nil {
			return fmt.Errorf("读取总结开关失败: %w", err)
		}
		if !enabled {
			return nil
		}
		if _, err := reviews.PlanTargetsForDay(now.AddDate(0, 0, -1).Format("2006-01-02")); err != nil {
			return fmt.Errorf("保存昨日计划目标失败: %w", err)
		}
		return nil
	})
}
