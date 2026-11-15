package main

import (
	"sync"

	studyapp "exam_notify/internal/app"
)

// admitResult 表示一条提醒在提醒互斥门下的投递方式。
type admitResult int

const (
	// admitDeliver 完整投递：响铃、系统通知与前端事件。
	admitDeliver admitResult = iota
	// admitUpdate 同类型任务/休息提醒：仅发事件更新弹窗内容，铃声继续、不重复通知。
	admitUpdate
	// admitDrop 忽略或已排队，不投递。
	admitDrop
)

// reminderGate 保证同一时刻只展示一个提醒弹窗：
// - 无弹窗时提醒直接展示；
// - 同类型 activity 弹窗未关时忽略后续触发（只弹一次、不补弹）；
// - 同类型 plan/break 弹窗未关时仅更新弹窗内容；
// - 不同类型排队，当前弹窗关闭后按先后顺序立即展示，同类型只保留最新一条；
// - 每日复习弹窗优先级最高：复习未完成期间提醒一律排队，复习完成后恢复上述规则。
type reminderGate struct {
	mu         sync.Mutex
	reviewHeld bool
	active     string
	pending    []studyapp.Reminder
}

// admit 判断一条提醒如何投递，并在需要排队时把提醒放入队列。
func (g *reminderGate) admit(reminder studyapp.Reminder) admitResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	// 复习弹窗未完成时所有提醒排队，等复习完成后再按既有规则投递。
	if g.reviewHeld {
		g.queue(reminder)
		return admitDrop
	}
	switch {
	case g.active == "":
		g.active = reminder.Kind
		return admitDeliver
	case g.active == reminder.Kind:
		if reminder.Kind == "activity" {
			return admitDrop
		}
		return admitUpdate
	default:
		g.queue(reminder)
		return admitDrop
	}
}

// queue 把提醒放入等待队列：同类型只保留最新一条，并保持原有位置。
func (g *reminderGate) queue(reminder studyapp.Reminder) {
	for i := range g.pending {
		if g.pending[i].Kind == reminder.Kind {
			g.pending[i] = reminder
			return
		}
	}
	g.pending = append(g.pending, reminder)
}

// release 关闭当前弹窗，返回应立即投递的下一条排队提醒；无排队时返回 nil。
// 复习弹窗未完成时即使当前弹窗被关闭也不投递，等待复习完成。
func (g *reminderGate) release() *studyapp.Reminder {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == "" {
		return nil
	}
	g.active = ""
	if g.reviewHeld || len(g.pending) == 0 {
		return nil
	}
	next := g.pending[0]
	g.pending = g.pending[1:]
	g.active = next.Kind
	return &next
}

// holdReview 标记每日复习弹窗正在展示：后续提醒一律排队，不打断复习。
func (g *reminderGate) holdReview() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reviewHeld = true
}

// releaseReview 标记每日复习完成，返回应立即投递的下一条排队提醒；
// 仍有其他弹窗展示时返回 nil，等该弹窗关闭后按序投递。
func (g *reminderGate) releaseReview() *studyapp.Reminder {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reviewHeld = false
	if g.active != "" || len(g.pending) == 0 {
		return nil
	}
	next := g.pending[0]
	g.pending = g.pending[1:]
	g.active = next.Kind
	return &next
}
