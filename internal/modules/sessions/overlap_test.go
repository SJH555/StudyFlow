package sessions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManualPlanWarningUsesOnlyStillAvailableTime(t *testing.T) {
	db := sessionTestDB(t) // 09:00–12:00
	day := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	in := ManualInput{SubjectID: 1, StartAt: day.Format(time.RFC3339), EndAt: day.Add(15 * time.Minute).Format(time.RFC3339)}
	_, _, err := addManualAt(db, in, day.Add(-time.Hour))
	var warning *ManualPlanConflict
	if !errors.As(err, &warning) {
		t.Fatalf("expected warning, got %v", err)
	}
	in.PlanConflictToken = warning.Token
	in.EndAt = day.Add(16 * time.Minute).Format(time.RFC3339)
	_, _, err = addManualAt(db, in, day)
	var changed *ManualPlanConflict
	if !errors.As(err, &changed) || changed.Token == warning.Token {
		t.Fatalf("changed interval should require fresh confirmation: %v", err)
	}
	in.PlanConflictToken = ""
	if _, _, err := addManualAt(db, in, day.Add(16*time.Minute)); err != nil {
		t.Fatalf("past interval with time remaining in the plan should not warn: %v", err)
	}
	in.StartAt, in.EndAt = day.Add(30*time.Minute).Format(time.RFC3339), day.Add(time.Hour).Format(time.RFC3339)
	if _, _, err := addManualAt(db, in, day.Add(2*time.Hour)); err != nil {
		t.Fatalf("ended plan should not warn: %v", err)
	}
}

func TestResumeChecksUnboundedLegacyTimer(t *testing.T) {
	db := sessionTestDB(t)
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	timestamp := func(offset time.Duration) string { return now.Add(offset).Format(time.RFC3339) }
	_, err := db.Exec(`INSERT INTO sessions(id,subject_id,start_at,paused_at,duration_seconds)
		VALUES(1,1,?,?,1200)`, timestamp(-time.Hour), timestamp(-40*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := addManualAt(db, ManualInput{SubjectID: 1, StartAt: timestamp(time.Minute), EndAt: timestamp(11 * time.Minute)}, now.Add(-time.Hour)); err == nil {
		t.Fatal("future interval should warn about the plan")
	} else {
		var warning *ManualPlanConflict
		if !errors.As(err, &warning) {
			t.Fatal(err)
		}
		_, _, err = addManualAt(db, ManualInput{SubjectID: 1, StartAt: timestamp(time.Minute),
			EndAt: timestamp(11 * time.Minute), PlanConflictToken: warning.Token}, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ChangeTimer(db, "1", "resume", now); err == nil || !strings.Contains(err.Error(), "时间冲突") {
		t.Fatalf("unbounded resume accepted: %v", err)
	}
	if _, err := ChangeTimer(db, "1", "resume", now.Add(11*time.Minute)); err != nil {
		t.Fatalf("adjacent resume blocked: %v", err)
	}
}
