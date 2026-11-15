package app

import (
	"sync"

	"exam_notify/internal/modules/review"
)

// Reminder 描述一条待投递提醒的类型、负载、标题与正文。
type Reminder struct {
	Kind, Payload, Title, Body string
}

// Dispatch 表示一次投递结果：包含待展示的每日总结或提醒。
type Dispatch struct {
	Review   *review.DailyReview
	Reminder *Reminder
}

// ReminderQueue 保存临时投递状态，零值即可直接使用。
type ReminderQueue struct {
	mu      sync.Mutex
	waiting []Reminder
	active  bool
}

// Enqueue 将提醒放入队列：存在待办总结或队列已占用时排队，否则立即投递。
func (q *ReminderQueue) Enqueue(reminder Reminder, pendingReview func() (*review.DailyReview, error)) (Dispatch, error) {
	pending, err := pendingReview()
	if err != nil {
		return Dispatch{}, err
	}
	q.mu.Lock()
	queued := pending != nil || q.active || len(q.waiting) != 0
	if queued {
		// 替换同类型的等待提醒并将其移到队尾。
		for i := len(q.waiting) - 1; i >= 0; i-- {
			if q.waiting[i].Kind == reminder.Kind {
				q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
				break
			}
		}
		q.waiting = append(q.waiting, reminder)
	}
	q.mu.Unlock()

	if pending != nil {
		return Dispatch{Review: pending}, nil
	}
	if queued {
		return q.Release(pendingReview)
	}
	return Dispatch{Reminder: &reminder}, nil
}

// Acknowledge 标记当前提醒已被确认，允许投递下一条。
func (q *ReminderQueue) Acknowledge() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.active = false
}

// Release 在队列非空且当前无投递中的提醒时，取出队首提醒进行投递。
func (q *ReminderQueue) Release(pendingReview func() (*review.DailyReview, error)) (Dispatch, error) {
	pending, err := pendingReview()
	if err != nil {
		return Dispatch{}, err
	}
	if pending != nil {
		return Dispatch{Review: pending}, nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.waiting) == 0 {
		q.active = false
		return Dispatch{}, nil
	}
	if q.active {
		return Dispatch{}, nil
	}
	reminder := q.waiting[0]
	q.waiting = q.waiting[1:]
	q.active = true
	return Dispatch{Reminder: &reminder}, nil
}
