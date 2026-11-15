package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManualPauseStartsBreakAndDayPauseDoesNot(t *testing.T) {
	store := breakTestStore(t)
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO sessions(id,subject_id,plan_id,start_at,source) VALUES(1,1,1,?,'timer')`, time.Now().Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pause-day", "resume", "pause"} {
		response := httptest.NewRecorder()
		testSessionAction(store, response, httptest.NewRequest(http.MethodPost, "/sessions/1", strings.NewReader(`{"action":"`+action+`"}`)), "1")
		if response.Code != http.StatusOK {
			t.Fatalf("%s failed: %d %s", action, response.Code, response.Body.String())
		}
		var breakStarted sql.NullString
		if err := testQueryRow(store, `SELECT break_started_at FROM sessions WHERE id=1`).Scan(&breakStarted); err != nil {
			t.Fatal(err)
		}
		if action == "pause" && !breakStarted.Valid {
			t.Fatal("manual pause did not start break")
		}
		if action != "pause" && breakStarted.Valid {
			t.Fatalf("%s started break", action)
		}
	}
	response := httptest.NewRecorder()
	testSessionAction(store, response, httptest.NewRequest(http.MethodPost, "/sessions/1", strings.NewReader(`{"action":"pause-day"}`)), "1")
	if response.Code != http.StatusOK {
		t.Fatalf("pause-day after break: %d %s", response.Code, response.Body.String())
	}
	var breakStarted sql.NullString
	if err := testQueryRow(store, `SELECT break_started_at FROM sessions WHERE id=1`).Scan(&breakStarted); err != nil {
		t.Fatal(err)
	}
	if breakStarted.Valid {
		t.Fatal("pausing all plans did not clear active break")
	}
}

func TestBreakBudgetCarriesAcrossPauses(t *testing.T) {
	store := breakTestStore(t)
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO sessions(id,subject_id,plan_id,start_at,source) VALUES(1,1,1,?,'timer')`, time.Now().Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	action := func(name string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		testSessionAction(store, response, httptest.NewRequest(http.MethodPost, "/sessions/1", strings.NewReader(`{"action":"`+name+`"}`)), "1")
		return response
	}
	if response := action("pause"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	started := time.Now().Add(-30 * time.Second).Format(time.RFC3339)
	if _, err := testExec(store, `UPDATE sessions SET break_started_at=? WHERE id=1`, started); err != nil {
		t.Fatal(err)
	}
	if response := action("resume"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var used int64
	if err := testQueryRow(store, `SELECT break_used_seconds FROM sessions WHERE id=1`).Scan(&used); err != nil || used < 30 {
		t.Fatalf("first break used=%d err=%v", used, err)
	}
	if response := action("pause"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if _, due, err := store.scheduler.BreakReminder(time.Now().Add(30 * time.Second)); err != nil || !due {
		t.Fatalf("remaining break reminder due=%v err=%v", due, err)
	}
	if response := action("resume"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if _, err := testExec(store, `UPDATE sessions SET break_used_seconds=60 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if response := action("pause"); response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "休息时间已用完") {
		t.Fatalf("exhausted break accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestBreakBudgetCarriesAcrossEndedSessions(t *testing.T) {
	store := breakTestStore(t)
	defer store.Close()
	now := time.Now()
	previousStart := now.Add(-20 * time.Minute)
	previousEnd := now.Add(-10 * time.Minute)
	if previousStart.Day() != now.Day() {
		previousStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		previousEnd = now
	}
	if _, err := testExec(store, `INSERT INTO sessions(id,subject_id,plan_id,start_at,end_at,break_used_seconds,source) VALUES(1,1,1,?,?,30,'timer')`, previousStart.Format(time.RFC3339), previousEnd.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	action := func(id string, body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		testSessionAction(store, response, httptest.NewRequest(http.MethodPost, "/sessions/"+id, strings.NewReader(body)), id)
		return response
	}
	if response := action("start", `{"action":"start","subjectId":1,"planId":1}`); response.Code != http.StatusOK {
		t.Fatalf("restart failed: %d %s", response.Code, response.Body.String())
	}
	list := httptest.NewRecorder()
	testRoute(store, store.sessions, list, httptest.NewRequest(http.MethodGet, "/sessions?date="+now.Format("2006-01-02"), nil))
	if !strings.Contains(list.Body.String(), `"breakUsedSeconds":30`) {
		t.Fatalf("new session did not show previous break use: %s", list.Body.String())
	}
	if response := action("2", `{"action":"pause"}`); response.Code != http.StatusOK {
		t.Fatalf("pause failed: %d %s", response.Code, response.Body.String())
	}
	if _, err := testExec(store, `UPDATE sessions SET break_started_at=? WHERE id=2`, now.Add(-35*time.Second).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, due, err := store.scheduler.BreakReminder(time.Now()); err != nil || !due {
		t.Fatalf("remaining break reminder due=%v err=%v", due, err)
	}
	if response := action("2", `{"action":"stop"}`); response.Code != http.StatusOK {
		t.Fatalf("stop failed: %d %s", response.Code, response.Body.String())
	}
	var deleted int
	var used int64
	if err := testQueryRow(store, `SELECT deleted,break_used_seconds FROM sessions WHERE id=2`).Scan(&deleted, &used); err != nil || deleted != 1 || used < 35 {
		t.Fatalf("short session lost break use: deleted=%d used=%d err=%v", deleted, used, err)
	}
	if response := action("start", `{"action":"start","subjectId":1,"planId":1}`); response.Code != http.StatusOK {
		t.Fatalf("second restart failed: %d %s", response.Code, response.Body.String())
	}
	if response := action("3", `{"action":"pause"}`); response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "休息时间已用完") {
		t.Fatalf("used break budget reset after restart: %d %s", response.Code, response.Body.String())
	}
}

func breakTestStore(t *testing.T) *App {
	t.Helper()
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(store, `INSERT INTO plans(id,subject_id,start_time,duration_minutes,break_minutes) VALUES(1,1,'09:00',180,1)`); err != nil {
		t.Fatal(err)
	}
	return store
}
