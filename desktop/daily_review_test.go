package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
