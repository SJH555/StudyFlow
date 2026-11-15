package schedule

import (
	"testing"
	"time"
)

func TestPlanConflictCrossesWeekBoundary(t *testing.T) {
	aDays, _ := ParseWeekdays("6")
	bDays, _ := ParseWeekdays("0")
	if _, overlap := Overlap(23*60+30, 90, aDays, 10, 30, bDays); !overlap {
		t.Fatal("Saturday plan ending Sunday did not conflict")
	}
}

func TestScheduledPlanEndAcrossMidnight(t *testing.T) {
	started := time.Date(2026, 11, 2, 23, 45, 0, 0, time.Local)
	end, ok := ScheduledEnd(started, "23:30", 90, "1")
	if !ok || !end.Equal(time.Date(2026, 11, 3, 1, 0, 0, 0, time.Local)) {
		t.Fatalf("end=%s found=%t", end, ok)
	}
}

func TestBoundaryEndAnchorsEarlyStartToUpcomingWindow(t *testing.T) {
	// 提前 30 分钟开始：12:00 开始 12:30–13:30 的计划，应锚定到 13:30。
	moment := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	end, ok := BoundaryEnd(moment, "12:30", 60, "4")
	if !ok || !end.Equal(time.Date(2026, 11, 5, 13, 30, 0, 0, time.Local)) {
		t.Fatalf("end=%s found=%t", end, ok)
	}
}

func TestBoundaryEndKeepsInWindowAndAfterWindowSemantics(t *testing.T) {
	// 时段内开始仍返回时段结束。
	inWindow := time.Date(2026, 11, 5, 13, 0, 0, 0, time.Local)
	if end, ok := BoundaryEnd(inWindow, "12:30", 60, "4"); !ok || !end.Equal(time.Date(2026, 11, 5, 13, 30, 0, 0, time.Local)) {
		t.Fatalf("in-window end=%s found=%t", end, ok)
	}
	// 时段结束后不再给出边界，保持无自动结束的既有语义。
	afterWindow := time.Date(2026, 11, 5, 14, 0, 0, 0, time.Local)
	if _, ok := BoundaryEnd(afterWindow, "12:30", 60, "4"); ok {
		t.Fatal("after-window start unexpectedly returned a boundary")
	}
}
