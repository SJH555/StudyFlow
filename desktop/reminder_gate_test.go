package main

import (
	"testing"

	studyapp "exam_notify/internal/app"
)

func TestReminderGateDeliversFirstReminderImmediately(t *testing.T) {
	var g reminderGate
	if got := g.admit(studyapp.Reminder{Kind: "plan"}); got != admitDeliver {
		t.Fatalf("admit = %v, want deliver", got)
	}
}

func TestReminderGateDropsRepeatedActivityWhileActive(t *testing.T) {
	var g reminderGate
	g.admit(studyapp.Reminder{Kind: "activity"})
	if got := g.admit(studyapp.Reminder{Kind: "activity"}); got != admitDrop {
		t.Fatalf("repeated activity admit = %v, want drop", got)
	}
}

func TestReminderGateUpdatesSameKindPlanWhileActive(t *testing.T) {
	var g reminderGate
	g.admit(studyapp.Reminder{Kind: "plan"})
	if got := g.admit(studyapp.Reminder{Kind: "plan"}); got != admitUpdate {
		t.Fatalf("repeated plan admit = %v, want update", got)
	}
}

func TestReminderGateQueuesDifferentKindsAndReleasesInOrder(t *testing.T) {
	var g reminderGate
	g.admit(studyapp.Reminder{Kind: "plan"})
	if got := g.admit(studyapp.Reminder{Kind: "activity"}); got != admitDrop {
		t.Fatalf("queued admit = %v, want drop", got)
	}
	if got := g.admit(studyapp.Reminder{Kind: "break"}); got != admitDrop {
		t.Fatalf("queued admit = %v, want drop", got)
	}
	next := g.release()
	if next == nil || next.Kind != "activity" {
		t.Fatalf("first released = %+v", next)
	}
	next = g.release()
	if next == nil || next.Kind != "break" {
		t.Fatalf("second released = %+v", next)
	}
	if next = g.release(); next != nil {
		t.Fatalf("unexpected release = %+v", next)
	}
}

func TestReminderGateReplacesQueuedSameKindInPlace(t *testing.T) {
	var g reminderGate
	g.admit(studyapp.Reminder{Kind: "plan"})
	g.admit(studyapp.Reminder{Kind: "activity", Body: "a1"})
	g.admit(studyapp.Reminder{Kind: "break"})
	g.admit(studyapp.Reminder{Kind: "activity", Body: "a2"})
	next := g.release()
	if next == nil || next.Kind != "activity" || next.Body != "a2" {
		t.Fatalf("replaced release = %+v", next)
	}
	next = g.release()
	if next == nil || next.Kind != "break" {
		t.Fatalf("second release = %+v", next)
	}
}

// 复习未完成期间提醒一律排队，复习完成后按先后顺序投递。
func TestReminderGateHoldsRemindersDuringReview(t *testing.T) {
	var g reminderGate
	g.holdReview()
	if got := g.admit(studyapp.Reminder{Kind: "plan"}); got != admitDrop {
		t.Fatalf("admit during review = %v, want drop", got)
	}
	if got := g.admit(studyapp.Reminder{Kind: "activity"}); got != admitDrop {
		t.Fatalf("admit during review = %v, want drop", got)
	}
	next := g.releaseReview()
	if next == nil || next.Kind != "plan" {
		t.Fatalf("first after review = %+v, want plan", next)
	}
	next = g.release()
	if next == nil || next.Kind != "activity" {
		t.Fatalf("second after review = %+v, want activity", next)
	}
	if next = g.release(); next != nil {
		t.Fatalf("unexpected release = %+v", next)
	}
}

// 复习期间同类型排队提醒只保留最新一条。
func TestReminderGateReplacesQueuedSameKindDuringReview(t *testing.T) {
	var g reminderGate
	g.holdReview()
	g.admit(studyapp.Reminder{Kind: "plan", Body: "p1"})
	g.admit(studyapp.Reminder{Kind: "plan", Body: "p2"})
	next := g.releaseReview()
	if next == nil || next.Body != "p2" {
		t.Fatalf("release after review = %+v, want latest plan", next)
	}
}

// 复习接管时已有弹窗不关闭，复习完成后等其关闭再投递排队提醒。
func TestReminderGateKeepsActiveDialogAcrossReview(t *testing.T) {
	var g reminderGate
	g.admit(studyapp.Reminder{Kind: "plan"})
	g.holdReview()
	g.admit(studyapp.Reminder{Kind: "activity"})
	if next := g.releaseReview(); next != nil {
		t.Fatalf("release while dialog active = %+v, want nil", next)
	}
	next := g.release()
	if next == nil || next.Kind != "activity" {
		t.Fatalf("release after dialog closed = %+v, want activity", next)
	}
}

// 复习未完成时关闭当前弹窗也不投递排队提醒，复习完成后直接补上。
func TestReminderGateReleaseDuringReviewDoesNotDeliver(t *testing.T) {
	var g reminderGate
	g.admit(studyapp.Reminder{Kind: "plan"})
	g.holdReview()
	g.admit(studyapp.Reminder{Kind: "activity"})
	if next := g.release(); next != nil {
		t.Fatalf("release during review = %+v, want nil", next)
	}
	next := g.releaseReview()
	if next == nil || next.Kind != "activity" {
		t.Fatalf("release after review = %+v, want activity", next)
	}
}
