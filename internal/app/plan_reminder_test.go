package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func TestSchedulerPlanReminderUsesCurrentPlanAfterMissedPlan(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04'),(2,'英语','#34C759','2026-11-04'),(3,'物理','#FF9500','2026-11-04')`)
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'09:00',30),(2,2,'09:40',20),(3,3,'10:30',30)`)
	start := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	schedulerExec(t, database, `INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at,source) VALUES(1,1,1,?,?,'timer')`, start.Format(time.RFC3339), start.Add(30*time.Minute).Format(time.RFC3339))

	at := time.Date(2026, 11, 5, 10, 30, 0, 0, time.Local)
	alert, err := scheduler.PlanReminder(at, false)
	if err != nil {
		t.Fatal(err)
	}
	if alert == nil || alert.Ended == nil || alert.Ended.Name != "数学" {
		t.Fatalf("ended = %+v", alert)
	}
	if alert.Started == nil || alert.Started.Name != "物理" {
		t.Fatalf("started = %+v", alert)
	}
	if text := PlanAlertText(*alert); !strings.Contains(text, "数学") || !strings.Contains(text, "物理") || strings.Contains(text, "英语") {
		t.Fatalf("unexpected reminder: %q", text)
	}
}

func TestSchedulerPlanReminderContinuesAfterFinishError(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04'),(2,'物理','#FF9500','2026-11-04')`)
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'09:00',30),(2,2,'10:30',30)`)
	// 非法 planned_end_at 会让 FinishExpired 解析失败，但不应阻止后续的 start 查询。
	schedulerExec(t, database, `INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at,source) VALUES(1,1,1,'2026-11-05T09:00:00+08:00','not-a-time','timer')`)

	at := time.Date(2026, 11, 5, 10, 30, 0, 0, time.Local)
	alert, err := scheduler.PlanReminder(at, true)
	if err == nil {
		t.Fatal("expected finish error to be reported")
	}
	if alert == nil || alert.Started == nil || alert.Started.Name != "物理" {
		t.Fatalf("start plan was not found despite finish error: %+v", alert)
	}
	if alert.Ended != nil {
		t.Fatalf("unexpected completion: %+v", alert.Ended)
	}
}

func TestPlanReminderSkipsStartedPlanWithActiveTimer(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`)
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'09:00',30)`)
	// 08:30 提前开始，无 planned_end_at（历史数据形态），计时仍在进行。
	schedulerExec(t, database, `INSERT INTO sessions(id,subject_id,plan_id,start_at,source) VALUES(1,1,1,'2026-11-05T08:30:00+08:00','timer')`)

	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	alert, err := scheduler.PlanReminder(at, true)
	if err != nil {
		t.Fatal(err)
	}
	if alert != nil {
		t.Fatalf("started plan with active timer still reminded: %+v", alert)
	}
}
