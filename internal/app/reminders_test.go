package app

import (
	"errors"
	"testing"

	"exam_notify/internal/modules/review"
)

func TestReminderQueuePrioritizesReviewAndReplacesWaitingKind(t *testing.T) {
	var q ReminderQueue
	pending := &review.DailyReview{Day: "2026-11-05"}
	lookup := func() (*review.DailyReview, error) { return pending, nil }
	for _, notice := range []Reminder{
		{Kind: "plan", Payload: "old"},
		{Kind: "activity", Payload: "activity"},
		{Kind: "plan", Payload: "new"},
	} {
		result, err := q.Enqueue(notice, lookup)
		if err != nil || result.Review != pending || result.Reminder != nil {
			t.Fatalf("expected review priority, got %+v, %v", result, err)
		}
	}
	pending = nil
	result, err := q.Release(lookup)
	if err != nil || result.Reminder == nil || result.Reminder.Payload != "activity" {
		t.Fatalf("expected activity first, got %+v, %v", result, err)
	}
	result, err = q.Release(lookup)
	if err != nil || result.Reminder != nil || result.Review != nil {
		t.Fatalf("released while awaiting acknowledgement: %+v, %v", result, err)
	}
	q.Acknowledge()
	result, err = q.Release(lookup)
	if err != nil || result.Reminder == nil || result.Reminder.Payload != "new" {
		t.Fatalf("expected replacement plan, got %+v, %v", result, err)
	}
	q.Acknowledge()
	result, err = q.Release(lookup)
	if err != nil || result.Reminder != nil || result.Review != nil {
		t.Fatalf("queue should be empty: %+v, %v", result, err)
	}
}

func TestReminderQueueLookupFailureDoesNotEnqueue(t *testing.T) {
	var q ReminderQueue
	failure := errors.New("review unavailable")
	_, err := q.Enqueue(Reminder{Kind: "plan"}, func() (*review.DailyReview, error) { return nil, failure })
	if !errors.Is(err, failure) {
		t.Fatalf("enqueue error = %v", err)
	}
	result, err := q.Release(func() (*review.DailyReview, error) { return nil, nil })
	if err != nil || result.Reminder != nil || result.Review != nil {
		t.Fatalf("failed lookup unexpectedly queued reminder: %+v, %v", result, err)
	}
}
