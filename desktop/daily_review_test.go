package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/review"
	"exam_notify/internal/modules/settings"
	"exam_notify/internal/shared/studycalendar"
)

func TestDailyReviewSwitchStartsTodayAndStopsCalculation(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enabled, _, _, start, err := store.reviewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if enabled || start != time.Now().Format("2006-01-02") {
		t.Fatalf("wrong defaults: enabled=%v start=%s", enabled, start)
	}
	if _, err = testExec(store, `UPDATE settings SET value='2000-01-01' WHERE key='dailyReviewStartDate'`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"dailyReviewEnabled":"1"}`))
	response := httptest.NewRecorder()
	testRoute(store, store.handleAPI, response, request)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	enabled, _, _, start, err = store.reviewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || start != time.Now().Format("2006-01-02") {
		t.Fatalf("enable did not reset start: enabled=%v start=%s", enabled, start)
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if _, err = testExec(store, `UPDATE settings SET value=? WHERE key='dailyReviewStartDate'`, yesterday); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"dailyReviewEnabled":"1","dailyReviewMode":"project","dailyReviewThreshold":"60"}`))
	response = httptest.NewRecorder()
	testRoute(store, store.handleAPI, response, request)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	enabled, _, _, start, err = store.reviewConfig()
	if err != nil || !enabled || start != yesterday {
		t.Fatalf("editing enabled review reset start: enabled=%v start=%s err=%v", enabled, start, err)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"dailyReviewEnabled":"0"}`))
	response = httptest.NewRecorder()
	testRoute(store, store.handleAPI, response, request)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	enabled, _, _, _, err = store.reviewConfig()
	if err != nil || enabled {
		t.Fatalf("disable failed: %v %v", enabled, err)
	}
}

func TestDailyReviewPerProjectOnlyListsMissedTargets(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	day := time.Now().AddDate(0, 0, -1)
	for _, subject := range []struct {
		id   int
		name string
	}{{1, "数学"}, {2, "英语"}} {
		if _, err = testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(?,?,?,?)`, subject.id, subject.name, "#123456", day.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		if _, err = testExec(store, `INSERT INTO plans(subject_id,start_time,duration_minutes,break_minutes,weekdays,active,color) VALUES(?,'09:00',60,0,'0,1,2,3,4,5,6',1,'#123456')`, subject.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = testExec(store, `INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds) VALUES(1,?,?,1800)`, day.Format(time.RFC3339), day.Add(30*time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	review, err := store.reviewForDay(day.Format("2006-01-02"), "project", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Items) != 1 || review.Items[0].SubjectName != "英语" {
		t.Fatalf("expected only English reason: %#v", review.Items)
	}
}

func TestReviewRuleChangeSettlesPastDaysWithOldStandard(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now().In(studycalendar.Location)
	first := studycalendar.Day(now.AddDate(0, 0, -2))
	yesterday := studycalendar.Day(now.AddDate(0, 0, -1))
	reviewResultsFixture(t, app, first, "total", "23", "59", first, yesterday)
	if _, err := testExec(app, `INSERT INTO subject_versions(subject_id,effective_day,name,color,created_at,status)
			VALUES(1,?,'数学','#007AFF',?,'active')`, first, first); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(app, `INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source)
			VALUES(1,?,?,2700,'manual')`, yesterday+"T09:00:00+08:00", yesterday+"T09:45:00+08:00"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	err := app.database.WithAccess(ctx, func(access *persistence.Access) error {
		db := access.Queries(ctx)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/settings",
			strings.NewReader(`{"dailyReviewThreshold":"80","dailyReviewMode":"project"}`))
		app.settings(db, response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("saving rule: %s", response.Body.String())
		}
		service := review.New(db)
		history, err := service.History()
		if err != nil {
			return err
		}
		if len(history) != 1 || history[0].Day != yesterday || history[0].Status != "passed" ||
			history[0].Threshold != 70 || history[0].Mode != "total" || history[0].ActualSeconds != 2700 {
			t.Fatalf("yesterday should be stored as passed under the old rule: %+v", history)
		}
		check := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 0, 0, now.Location())
		pending, err := service.Pending(check)
		if err != nil {
			return err
		}
		if pending == nil || pending.Day != first || pending.Threshold != 70 || pending.Mode != "total" ||
			len(pending.Items) != 1 || pending.Items[0].SubjectID != 0 {
			t.Fatalf("unfilled review should retain the old rule: %+v", pending)
		}
		// 重新开放检查后补录不会改写已结算的数据，也不会使达标日期补弹框。
		if _, err := db.Exec(`UPDATE settings SET value='0' WHERE key IN ('dailyReviewCheckHour','dailyReviewCheckMinute')`); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE sessions SET deleted=1`); err != nil {
			return err
		}
		if err := service.Submit(first, []review.ReviewItem{{SubjectID: 0, Reason: "休息"}}); err != nil {
			return err
		}
		if next, err := service.Pending(check); err != nil || next != nil {
			t.Fatalf("passed day must never become pending: %+v, %v", next, err)
		}
		history, err = service.History()
		if err != nil {
			return err
		}
		if len(history) != 2 || history[0].ActualSeconds != 2700 || history[1].Status != "submitted" ||
			len(history[1].Items) != 1 || history[1].Items[0].Reason != "休息" {
			t.Fatalf("unexpected stored history: %+v", history)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVersionedReviewSettlesAndSubmitsBulk(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now().In(studycalendar.Location)
	first := studycalendar.Day(now.AddDate(0, 0, -4))
	last := studycalendar.Day(now.AddDate(0, 0, -1))
	reviewResultsFixture(t, app, first, "project", "0", "0", first, last)
	ctx := context.Background()
	err := app.database.WithAccess(ctx, func(access *persistence.Access) error {
		db := access.Queries(ctx)
		service := review.New(db)
		pending, err := service.Pending(now)
		if err != nil {
			return err
		}
		if pending == nil || pending.Day != first || pending.Status != "pending" {
			t.Fatalf("earliest date should be saved when checked: %+v", pending)
		}
		if count, err := service.SaveBulk(now, first, last, "休息"); err != nil || count != 4 {
			t.Fatalf("bulk submit count=%d, err=%v", count, err)
		}
		history, err := service.History()
		if err != nil {
			return err
		}
		if len(history) != 4 || history[0].Status != "submitted" || len(history[0].Items) != 1 ||
			history[0].Items[0].Reason != "休息" {
			t.Fatalf("unexpected bulk history: %+v", history)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func reviewResultsFixture(t *testing.T, app *App, start, mode, hour, minute, first, last string) {
	t.Helper()
	if _, err := testExec(app, `INSERT INTO settings(key,value) VALUES
		('dailyReviewEnabled','1'),('dailyReviewMode',?),('dailyReviewThreshold','70'),
		('dailyReviewStartDate',?),('dailyReviewCheckHour',?),('dailyReviewCheckMinute',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, mode, start, hour, minute); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(app, `WITH RECURSIVE days(day) AS (SELECT ? UNION ALL SELECT date(day,'+1 day') FROM days WHERE day<?)
		INSERT INTO plan_versions(kind,effective_day,legacy_targets_json)
		SELECT 'legacy_target',day,'[{"subjectId":1,"subjectName":"数学","plannedSeconds":3600}]' FROM days`, first, last); err != nil {
		t.Fatal(err)
	}
}

func TestVersionedReviewReenableRestoresPendingWithoutBackfillingDisabledDays(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now().In(studycalendar.Location)
	firstDay := now.AddDate(0, 0, -3)
	first, yesterday := studycalendar.Day(firstDay), studycalendar.Day(now.AddDate(0, 0, -1))
	reviewResultsFixture(t, app, first, "total", "0", "0", first, yesterday)

	ctx := context.Background()
	err := app.database.WithAccess(ctx, func(access *persistence.Access) error {
		db := access.Queries(ctx)
		service := review.New(db)
		pending, err := service.Pending(firstDay.AddDate(0, 0, 1))
		if err != nil || pending == nil || pending.Day != first || pending.Status != "pending" {
			t.Fatalf("expected a stored pending review before disabling: %+v, %v", pending, err)
		}
		if err := settings.Save(db, map[string]string{"dailyReviewEnabled": "0"}, settings.Actions{}); err != nil {
			return err
		}
		if pending, err := service.Pending(now); err != nil || pending != nil {
			t.Fatalf("review still offered while disabled: %+v, %v", pending, err)
		}
		if err := settings.Save(db, map[string]string{"dailyReviewEnabled": "1"}, settings.Actions{}); err != nil {
			return err
		}
		pending, err = service.Pending(now)
		if err != nil || pending == nil || pending.Day != first || pending.Status != "pending" {
			t.Fatalf("previous pending review not restored: %+v, %v", pending, err)
		}
		if err := service.Submit(first, []review.ReviewItem{{SubjectID: 0, Reason: "休息"}}); err != nil {
			return err
		}
		if pending, err := service.Pending(now); err != nil || pending != nil {
			t.Fatalf("disabled days should not be backfilled: %+v, %v", pending, err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM daily_reviews").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("expected only the original review, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
