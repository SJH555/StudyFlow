package app

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/review"
)

func schedulerCount(t *testing.T, database *persistence.Database, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.WithRead(func(db *sql.DB, _ string) error {
		return db.QueryRow(query, args...).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func schedulerForDay(t *testing.T, database *persistence.Database, day, mode string, threshold int) *review.DailyReview {
	t.Helper()
	var result *review.DailyReview
	if err := database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		result, err = review.New(db).ForDay(day, mode, threshold)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func schedulerBulkOffer(t *testing.T, database *persistence.Database, now time.Time, pending *review.DailyReview) *review.BulkReviewOffer {
	t.Helper()
	var result *review.BulkReviewOffer
	if err := database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		result, err = review.New(db).BulkOffer(now, pending)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func schedulerSaveBulk(t *testing.T, database *persistence.Database, now time.Time, startDay, endDay, reason string) int {
	t.Helper()
	var count int
	if err := database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		count, err = review.New(db).SaveBulk(now, startDay, endDay, reason)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSchedulerPendingReviewThresholdCatchupAndPlanSnapshot(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	first := today.AddDate(0, 0, -2)
	second := today.AddDate(0, 0, -1)
	schedulerExec(t, database, `UPDATE settings SET value=? WHERE key='dailyReviewStartDate'`, first.Format("2006-01-02"))
	schedulerExec(t, database, `UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'`)
	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#123456',?)`, first.Format(time.RFC3339))
	schedulerExec(t, database, `INSERT INTO plans(id,subject_id,start_time,duration_minutes,break_minutes,weekdays,active,color) VALUES(1,1,'09:00',80,20,'0,1,2,3,4,5,6',1,'#123456')`)
	for _, item := range []struct {
		day     time.Time
		minutes int
	}{{first, 30}, {second, 29}} {
		schedulerExec(t, database, `INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds) VALUES(1,?,?,?)`, item.day.Format(time.RFC3339), item.day.Add(time.Duration(item.minutes)*time.Minute).Format(time.RFC3339), item.minutes*60)
	}
	pending, err := scheduler.PendingReview(today)
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.Day != second.Format("2006-01-02") {
		t.Fatalf("expected second day only, got %#v", pending)
	}
	if pending.PlannedSeconds != 3600 || pending.ActualSeconds != 29*60 {
		t.Fatalf("wrong totals: %#v", pending)
	}
	schedulerExec(t, database, `UPDATE plans SET deleted=1 WHERE id=1`)
	unchanged := schedulerForDay(t, database, second.Format("2006-01-02"), "total", 50)
	if unchanged.PlannedSeconds != 3600 {
		t.Fatalf("snapshot changed after plan deletion: %#v", unchanged)
	}
	beforeSix, err := scheduler.PendingReview(time.Date(today.Year(), today.Month(), today.Day(), 5, 59, 59, 0, today.Location()))
	if err != nil {
		t.Fatal(err)
	}
	if beforeSix != nil {
		t.Fatal("review appeared before 06:00")
	}
}

func TestSchedulerPendingReviewBulkReasonAfterFourDaysWithoutRecords(t *testing.T) {
	database, err := persistence.NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	first := today.AddDate(0, 0, -4)
	schedulerExec(t, database, `UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'`)
	schedulerExec(t, database, `UPDATE settings SET value=? WHERE key='dailyReviewStartDate'`, first.Format("2006-01-02"))
	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#123456',?)`, first.Format(time.RFC3339))
	schedulerExec(t, database, `INSERT INTO plans(subject_id,start_time,duration_minutes,weekdays,active,color) VALUES(1,'09:00',60,'0,1,2,3,4,5,6',1,'#123456')`)

	threeDayNow := today.AddDate(0, 0, -1)
	firstPending, checkErr := scheduler.PendingReview(threeDayNow)
	if checkErr != nil || firstPending == nil {
		t.Fatalf("three-day pending: %#v %v", firstPending, checkErr)
	}
	if tooEarly := schedulerBulkOffer(t, database, threeDayNow, firstPending); tooEarly != nil {
		t.Fatalf("bulk offer before day four: %#v", tooEarly)
	}
	pending, err := scheduler.PendingReview(today)
	if err != nil || pending == nil {
		t.Fatalf("pending: %#v %v", pending, err)
	}
	offer := schedulerBulkOffer(t, database, today, pending)
	if offer == nil {
		t.Fatalf("offer: %#v", offer)
	}
	if offer.DaysWithoutStudy != 4 || offer.PendingCount != 4 {
		t.Fatalf("wrong offer: %#v", offer)
	}
	count := schedulerSaveBulk(t, database, today, offer.StartDay, offer.EndDay, "生病休养")
	if count != 4 {
		t.Fatalf("bulk save: %d", count)
	}
	if reviews := schedulerCount(t, database, `SELECT count(*) FROM daily_reviews`); reviews != 4 {
		t.Fatalf("wrong saved reviews: %d", reviews)
	}
	if items := schedulerCount(t, database, `SELECT count(*) FROM daily_review_items WHERE reason='生病休养'`); items != 4 {
		t.Fatalf("wrong saved reasons: %d", items)
	}
	pending, err = scheduler.PendingReview(today)
	if err != nil || pending != nil {
		t.Fatalf("still pending: %#v %v", pending, err)
	}
}

func TestSchedulerPendingReviewReopenUsesSamePendingRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	database, err := persistence.NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(database)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")
	schedulerExec(t, database, `UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'`)
	schedulerExec(t, database, `UPDATE settings SET value=? WHERE key='dailyReviewStartDate'`, yesterday)
	schedulerExec(t, database, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#123456',?)`, yesterday)
	schedulerExec(t, database, `INSERT INTO plans(subject_id,start_time,duration_minutes,weekdays,active,color) VALUES(1,'09:00',60,'0,1,2,3,4,5,6',1,'#123456')`)
	pending, err := scheduler.PendingReview(today)
	if err != nil || pending == nil {
		t.Fatalf("expected pending before close: %#v %v", pending, err)
	}
	database.Close()

	database, err = persistence.NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler = NewScheduler(database)
	pending, err = scheduler.PendingReview(today)
	if err != nil || pending == nil || pending.Day != yesterday {
		t.Fatalf("pending changed after reopen: %#v %v", pending, err)
	}
	schedulerExec(t, database, `UPDATE settings SET value='0' WHERE key='dailyReviewEnabled'`)
	pending, err = scheduler.PendingReview(today)
	if err != nil || pending != nil {
		t.Fatalf("disabled review still pending: %#v %v", pending, err)
	}
	schedulerExec(t, database, `UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'`)
	schedulerExec(t, database, `UPDATE settings SET value=? WHERE key='dailyReviewStartDate'`, today.Format("2006-01-02"))
	pending, err = scheduler.PendingReview(today)
	if err != nil || pending != nil {
		t.Fatalf("old date returned after reenable: %#v %v", pending, err)
	}
}
