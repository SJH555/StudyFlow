package activities

import (
	"exam_notify/internal/infrastructure/persistence"

	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/sessions"
	"time"
)

// betweenActivityPlans 判断当前是否处于今日前一个计划结束后、后一个计划开始前的空档。
// 第一个计划之前与最后一个计划之后的时间不算空档。
func betweenActivityPlans(now time.Time, windows []plans.Window) bool {
	previous, next, active := false, false, false
	for _, window := range windows {
		if !now.Before(window.Start) && now.Before(window.End) {
			active = true
		}
		if !now.Before(window.End) {
			previous = true
		}
		if now.Before(window.Start) {
			next = true
		}
	}
	return !active && previous && next
}

// Resting 判断用户当前是否处于休息状态：先看是否有进行中的休息，再看是否处于计划间空档。
func Resting(db *persistence.Queries, now time.Time) (bool, error) {
	resting, err := sessions.HasActiveRest(db, now)
	if err != nil || resting {
		return resting, err
	}
	windows, err := plans.EffectiveWindows(db, now)
	if err != nil {
		return false, err
	}
	return betweenActivityPlans(now.Local(), windows), nil
}
