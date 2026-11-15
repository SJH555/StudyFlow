//go:build windows

package app

import (
	"exam_notify/internal/infrastructure/persistence"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsQADailySnapshotAndExactPlanHandoff(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := NewScheduler(database)
	at := time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)
	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'A','#007AFF','2026-11-01'),(2,'B','#34C759','2026-11-01')`)
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'09:00',30),(2,2,'09:30',30)`)
	if err := s.DailySnapshot(at); err != nil {
		t.Fatal(err)
	}
	if n := schedulerCount(t, database, `SELECT COUNT(*) FROM daily_plan_targets`); n != 0 {
		t.Fatalf("disabled snapshot wrote %d", n)
	}
	schedulerExec(t, database, `UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'`)
	if err := s.DailySnapshot(at); err != nil {
		t.Fatal(err)
	}
	if n := schedulerCount(t, database, `SELECT COUNT(*) FROM daily_plan_targets WHERE day='2026-11-14'`); n != 1 {
		t.Fatalf("yesterday snapshot count=%d", n)
	}
	schedulerExec(t, database, `UPDATE plans SET duration_minutes=90 WHERE id=2`)
	if err := s.DailySnapshot(at); err != nil {
		t.Fatal(err)
	}
	snap := schedulerForDay(t, database, "2026-11-14", "total", 50)
	if snap.PlannedSeconds != 3600 {
		t.Fatalf("snapshot was overwritten: %d", snap.PlannedSeconds)
	}
	start := time.Date(2026, 11, 15, 9, 0, 0, 0, time.Local)
	end := start.Add(30 * time.Minute)
	schedulerExec(t, database, `INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at,source) VALUES(1,1,1,?,?,'timer')`, start.Format(time.RFC3339), end.Format(time.RFC3339))
	alert, err := s.PlanReminder(end, true)
	if err != nil {
		t.Fatal(err)
	}
	if alert == nil || alert.Ended == nil || alert.Started == nil || alert.Ended.Name != "A" || alert.Started.Name != "B" {
		t.Fatalf("handoff=%+v", alert)
	}
	if n := schedulerCount(t, database, `SELECT COUNT(*) FROM sessions WHERE end_at IS NOT NULL AND duration_seconds=1800`); n != 1 {
		t.Fatalf("completed sessions=%d", n)
	}
}
