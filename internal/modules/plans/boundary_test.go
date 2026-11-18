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
	got, err := EndForStart(persistence.BindQueries(ctx, tx), 1, now)
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
	got, err := EndForStart(persistence.BindQueries(ctx, tx), 1, now)
	if err != nil || !got.Equal(want) {
		t.Fatalf("EndForStart = %v, %v; want %v", got, err, want)
	}
}

func TestReminderEarlyReturnReleasesSingleConnection(t *testing.T) {
	for _, kind := range []string{"starting", "current", "scan-error"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			owner, err := persistence.NewDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = owner.WithAccess(ctx, func(a *persistence.Access) error {
				q := a.Queries(ctx)
				if _, err := q.Exec(`INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18'); INSERT INTO plans(subject_id,start_time,duration_minutes,weekdays) VALUES(1,'09:00',60,'0,1,2,3,4,5,6')`); err != nil {
					return err
				}
				now := time.Date(2026, 11, 18, 9, 0, 0, 0, time.Local)
				if kind == "scan-error" {
					if _, err := q.Exec(`UPDATE plans SET duration_minutes='malformed'`); err != nil {
						return err
					}
					if _, err := CurrentAt(q, now); err == nil {
						t.Fatal("expected scan error")
					}
				} else {
					read := CurrentAt
					if kind == "starting" {
						read = StartingAt
					}
					item, err := read(q, now)
					if err != nil || item == nil {
						t.Fatalf("reminder = %v, %v", item, err)
					}
				}
				// This blocks until timeout if the early-return path leaked Rows.
				_, err := q.Exec(`UPDATE subjects SET name='available' WHERE id=1`)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
