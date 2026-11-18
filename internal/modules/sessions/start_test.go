package sessions

import (
	"exam_notify/internal/testutil"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func sessionTestDB(t *testing.T) *persistence.Queries {
	t.Helper()
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO plans(id,subject_id,start_time,duration_minutes,break_minutes) VALUES(1,1,'09:00',180,1)`); err != nil {
		t.Fatal(err)
	}
	return testutil.Borrow(t, db, path)
}

func TestStartPersistsBoundaryAndRollsBackOldTimerOnInsertFailure(t *testing.T) {
	db := sessionTestDB(t)
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	start := now.Add(-30 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO sessions(id,subject_id,start_at) VALUES(1,1,?)`, start); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(db, TimerInput{SubjectID: 999, PlanID: 1}, now); err == nil {
		t.Fatal("start with a missing subject succeeded")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sessions WHERE id=1 AND end_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("old timer not restored: count=%d err=%v", count, err)
	}

	id, err := Start(db, TimerInput{SubjectID: 1, PlanID: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	var plannedEnd, oldEnd string
	if err := db.QueryRow(`SELECT planned_end_at,(SELECT end_at FROM sessions WHERE id=1) FROM sessions WHERE id=?`, id).Scan(&plannedEnd, &oldEnd); err != nil {
		t.Fatal(err)
	}
	if plannedEnd != now.Add(2*time.Hour).Format(time.RFC3339) || oldEnd != now.Format(time.RFC3339) {
		t.Fatalf("planned end=%q old end=%q", plannedEnd, oldEnd)
	}
}

func TestStartAnchorsEarlyStartAndFinishesAtPlannedBoundary(t *testing.T) {
	db := sessionTestDB(t)
	// 计划时段 09:00–12:00，提前 1 小时在 08:00 开始。
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
	id, err := Start(db, TimerInput{SubjectID: 1, PlanID: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	var plannedEnd string
	if err := db.QueryRow(`SELECT planned_end_at FROM sessions WHERE id=?`, id).Scan(&plannedEnd); err != nil {
		t.Fatal(err)
	}
	if plannedEnd != want.Format(time.RFC3339) {
		t.Fatalf("planned end=%q, want %s", plannedEnd, want.Format(time.RFC3339))
	}
	// 到达计划结束时刻后自动结束，时长覆盖整段提前学习。
	ended, err := FinishExpired(db, want.Add(time.Minute))
	if err != nil || ended == nil || !ended.Saved || ended.Name != "数学" {
		t.Fatalf("completion = %+v, %v", ended, err)
	}
	var duration int64
	if err := db.QueryRow(`SELECT duration_seconds FROM sessions WHERE id=?`, id).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 4*3600 {
		t.Fatalf("duration=%d, want %d", duration, 4*3600)
	}
}
