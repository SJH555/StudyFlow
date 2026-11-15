package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestManualRecordCannotOverlapRunningPlanBeforeItsEnd(t *testing.T) {
	store := breakTestStore(t)
	defer store.Close()
	start := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	end := start.Add(3 * time.Hour)
	if _, err := testExec(store, `INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at,source) VALUES(1,1,1,?,?,'timer')`, start.Format(time.RFC3339), end.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	manualStart := start.Add(2 * time.Hour)
	manualEnd := manualStart.Add(30 * time.Minute)
	body := `{"subjectId":1,"startAt":"` + manualStart.Format(time.RFC3339) + `","endAt":"` + manualEnd.Format(time.RFC3339) + `"}`
	response := httptest.NewRecorder()
	testRoute(store, store.sessions, response, httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(body)))
	if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "时间冲突") {
		t.Fatalf("overlap accepted: %d %s", response.Code, response.Body.String())
	}
}
