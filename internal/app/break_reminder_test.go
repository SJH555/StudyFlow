package app

import (
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func TestSchedulerBreakReminderFiresOnceWhenDue(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`)
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes,break_minutes) VALUES(1,1,'09:00',180,1)`)
	now := time.Now().Truncate(time.Second)
	started := now.Add(-30 * time.Second).Format(time.RFC3339)
	schedulerExec(t, database, `INSERT INTO sessions(id,subject_id,plan_id,start_at,paused_at,break_started_at,source) VALUES(1,1,1,?,?,?,'timer')`, now.Add(-time.Hour).Format(time.RFC3339), started, started)

	if _, due, err := scheduler.BreakReminder(now); err != nil || due {
		t.Fatalf("reminder fired early: %t %v", due, err)
	}
	if subject, due, err := scheduler.BreakReminder(now.Add(31 * time.Second)); err != nil || !due || subject != "数学" {
		t.Fatalf("due reminder = %q %t %v", subject, due, err)
	}
	if _, due, err := scheduler.BreakReminder(now.Add(32 * time.Second)); err != nil || due {
		t.Fatalf("reminder repeated: %t %v", due, err)
	}
}
