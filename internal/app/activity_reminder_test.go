package app

import (
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func TestSchedulerActivityReminderSkipsOnlyRestAndGaps(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, `INSERT INTO settings(key,value) VALUES('activityReminderEnabled','1'),('activityInterval','1'),('activityStartTime','00:00'),('activitySkipRest','1')`)
	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`)
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes,break_minutes) VALUES(1,1,'09:00',180,1),(2,1,'14:00',60,0)`)

	at := func(hour, minute int) time.Time { return time.Date(2026, 11, 7, hour, minute, 0, 0, time.Local) }
	for _, tc := range []struct {
		hour, minute int
		fire         bool
	}{{8, 30, true}, {10, 0, true}, {12, 30, false}, {13, 59, false}, {14, 0, true}, {16, 0, true}} {
		got, err := scheduler.ActivityReminder(at(tc.hour, tc.minute))
		if err != nil || got != tc.fire {
			t.Fatalf("%02d:%02d fire=%v want=%v err=%v", tc.hour, tc.minute, got, tc.fire, err)
		}
	}

	now := at(10, 0)
	started := now.Add(-30 * time.Second).Format(time.RFC3339)
	schedulerExec(t, database, `INSERT INTO sessions(id,subject_id,plan_id,start_at,paused_at,break_started_at,source) VALUES(1,1,1,?,?,?,'timer')`, at(9, 0).Format(time.RFC3339), started, started)

	got, err := scheduler.ActivityReminder(now)
	if err != nil || got {
		t.Fatalf("paused break fire=%v err=%v", got, err)
	}
	got, err = scheduler.ActivityReminder(now.Add(31 * time.Second))
	if err != nil || !got {
		t.Fatalf("exhausted break fire=%v err=%v", got, err)
	}
}
