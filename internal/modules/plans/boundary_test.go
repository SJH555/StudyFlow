package plans

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func TestEndForStartReadsUncommittedPlanFromSuppliedTransaction(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-02')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO plans(id,subject_id,start_time,duration_minutes,weekdays) VALUES(1,1,'23:30',90,'1')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 23, 45, 0, 0, time.Local)
	want := time.Date(2026, 11, 3, 1, 0, 0, 0, time.Local)
	got, err := EndForStart(tx, 1, now)
	if err != nil || !got.Equal(want) {
		t.Fatalf("EndForStart = %v, %v; want %v", got, err, want)
	}
}

func TestEndForStartAnchorsEarlyStartToUpcomingWindow(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'12:30',60)`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	want := time.Date(2026, 11, 5, 13, 30, 0, 0, time.Local)
	got, err := EndForStart(tx, 1, now)
	if err != nil || !got.Equal(want) {
		t.Fatalf("EndForStart = %v, %v; want %v", got, err, want)
	}
}
