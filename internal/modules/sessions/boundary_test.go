package sessions

import (
	"database/sql"
	"testing"
	"time"
)

func TestExpiredPlanClosesAtBoundaryAndOnlyOnce(t *testing.T) {
	db := sessionTestDB(t)
	boundary := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	start := boundary.Add(-30 * time.Minute)
	if _, err := db.Exec(`INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at,source) VALUES(1,1,1,?,?,'timer')`, start.Format(time.RFC3339), boundary.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	ended, err := FinishExpired(db, boundary.Add(20*time.Minute))
	if err != nil || ended == nil || !ended.Saved || ended.Name != "数学" {
		t.Fatalf("completion = %+v, %v", ended, err)
	}
	var endRaw string
	var duration int64
	if err := db.QueryRow(`SELECT end_at,duration_seconds FROM sessions WHERE id=1`).Scan(&endRaw, &duration); err != nil {
		t.Fatal(err)
	}
	if endRaw != boundary.Format(time.RFC3339) || duration != 1800 {
		t.Fatalf("end=%s duration=%d", endRaw, duration)
	}
	if ended, err = FinishExpired(db, boundary.Add(21*time.Minute)); err != nil || ended != nil {
		t.Fatalf("duplicate completion = %+v, %v", ended, err)
	}
}

func TestExpiredPausedPlanUnderTenMinutesIsNotCounted(t *testing.T) {
	db := sessionTestDB(t)
	boundary := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	start := boundary.Add(-20 * time.Minute)
	paused := start.Add(8 * time.Minute)
	if _, err := db.Exec(`INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at,paused_at,duration_seconds,break_started_at,source) VALUES(1,1,1,?,?,?,?,?,'timer')`, start.Format(time.RFC3339), boundary.Format(time.RFC3339), paused.Format(time.RFC3339), 480, paused.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	ended, err := FinishExpired(db, boundary.Add(time.Minute))
	if err != nil || ended == nil || ended.Saved {
		t.Fatalf("completion = %+v, %v", ended, err)
	}
	var deleted, duration int
	var breakStarted sql.NullString
	if err := db.QueryRow(`SELECT deleted,duration_seconds,break_started_at FROM sessions WHERE id=1`).Scan(&deleted, &duration, &breakStarted); err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || duration != 480 || breakStarted.Valid {
		t.Fatalf("deleted=%d duration=%d break=%v", deleted, duration, breakStarted)
	}
}

func TestFinishExpiredFallsBackToPlanBoundaryForLegacySession(t *testing.T) {
	db := sessionTestDB(t)
	start := time.Date(2026, 11, 5, 11, 30, 0, 0, time.Local)
	if _, err := db.Exec(`INSERT INTO sessions(subject_id,plan_id,start_at) VALUES(1,1,?)`, start.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	boundary := start.Add(30 * time.Minute)
	if got, err := FinishExpired(db, boundary.Add(-time.Second)); err != nil || got != nil {
		t.Fatalf("completed before boundary: %+v, %v", got, err)
	}
	got, err := FinishExpired(db, boundary)
	if err != nil || got == nil || !got.Saved || got.EndAt != boundary.Format(time.RFC3339) {
		t.Fatalf("completion = %+v, %v", got, err)
	}
}
