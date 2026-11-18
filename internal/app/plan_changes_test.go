package app

import (
	"exam_notify/internal/testutil"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/review"
)

func TestApplyScheduleChangeFreezesBeforeChangeAndKeepsSnapshotOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	raw, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db := testutil.Borrow(t, raw, path)
	for _, query := range []string{
		"UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'",
		"UPDATE settings SET value='2026-11-05' WHERE key='dailyReviewStartDate'",
		"INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-05')",
		"INSERT INTO plans(subject_id,start_time,duration_minutes) VALUES(1,'09:00',60)",
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.Local)
	reviews := review.New(db)
	var saved string
	if err := ApplyScheduleChange(reviews, now, func() {
		if err := db.QueryRow("SELECT targets_json FROM daily_plan_targets WHERE day='2026-11-05'").Scan(&saved); err != nil {
			t.Fatalf("snapshot missing before change: %v", err)
		}
		if _, err := plans.Create(db, plans.Plan{}); err == nil {
			t.Fatal("invalid plan unexpectedly accepted")
		}
	}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := db.QueryRow("SELECT targets_json FROM daily_plan_targets WHERE day='2026-11-05'").Scan(&after); err != nil || after != saved || saved == "" {
		t.Fatalf("snapshot after rejected change = %q, %v; want %q", after, err, saved)
	}

	if _, err := db.Exec("UPDATE daily_plan_targets SET targets_json='invalid' WHERE day='2026-11-05'"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyScheduleChange(reviews, now, func() { t.Fatal("change ran after snapshot failure") }); err == nil {
		t.Fatal("expected snapshot error")
	}
}
