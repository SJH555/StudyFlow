package app

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/sessions"
)

// PlanAlert 携带一个已结束的学习时段和/或一个现在应开始的计划。
// 两者共用同一条提醒，因为结束到期的计划与开始下一个计划是同一次业务调用中的有序步骤。
type PlanAlert struct {
	Ended   *sessions.Completion `json:"ended,omitempty"`
	Started *plans.ReminderItem  `json:"started,omitempty"`
}

// PlanAlertText 渲染合并后的计划提醒文案。
func PlanAlertText(alert PlanAlert) string {
	if alert.Ended != nil && alert.Started != nil {
		return fmt.Sprintf("「%s」已结束；「%s」现在可以开始", alert.Ended.Name, alert.Started.Name)
	}
	if alert.Ended != nil {
		if !alert.Ended.Saved {
			return fmt.Sprintf("「%s」已结束，本次不足 10 分钟，未计入学习记录", alert.Ended.Name)
		}
		return fmt.Sprintf("「%s」已结束", alert.Ended.Name)
	}
	if alert.Started != nil {
		return fmt.Sprintf("「%s」现在可以开始", alert.Started.Name)
	}
	return ""
}

// PlanReminder 结束到期的计划，并在 checkStart 为真时查找现在开始的计划。
// 若某计划已结束但没有现在开始的计划，则回退到当前正在进行的计划。
// 没有任何内容需要展示时返回 nil。
//
// 每个阶段都会记录自己的错误并继续执行，因此某一阶段失败不会压制由其他阶段构建出的提醒。
// 桌面宿主持有按分钟去重的状态，仅在需要做开始检查时才传入 checkStart；
// 它必须在调用前消费掉自己的去重状态。
func (s *Scheduler) PlanReminder(now time.Time, checkStart bool) (*PlanAlert, error) {
	var completed *sessions.Completion
	var starting *plans.ReminderItem
	var errs []error

	if err := s.db.WithWrite(func(db *sql.DB, _ string) error {
		var err error
		completed, err = sessions.FinishExpired(db, now)
		return err
	}); err != nil {
		errs = append(errs, fmt.Errorf("结束到期学习计划失败: %w", err))
	}

	if checkStart {
		if err := s.db.WithRead(func(db *sql.DB, _ string) error {
			var err error
			starting, err = plans.StartingAt(db, now)
			return err
		}); err != nil {
			errs = append(errs, fmt.Errorf("检查学习计划提醒失败: %w", err))
		}
	}

	if completed != nil && starting == nil {
		if err := s.db.WithRead(func(db *sql.DB, _ string) error {
			var err error
			starting, err = plans.CurrentAt(db, now)
			return err
		}); err != nil {
			errs = append(errs, fmt.Errorf("检查当前学习计划失败: %w", err))
		}
	}

	if completed == nil && starting == nil {
		return nil, errors.Join(errs...)
	}
	return &PlanAlert{Ended: completed, Started: starting}, errors.Join(errs...)
}
