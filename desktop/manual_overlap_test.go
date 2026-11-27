package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManualRecordRejectsOverlapsAcrossSources(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(store, `INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source) VALUES
		(1,'2026-11-04T08:45:00+08:00','2026-11-04T09:45:00+08:00',3600,'manual'),
		(1,'2026-11-04T10:00:00+08:00','2026-11-04T11:00:00+08:00',3600,'timer')`); err != nil {
		t.Fatal(err)
	}
	for _, interval := range [][2]string{{"09:00", "09:30"}, {"10:30", "10:45"}} {
		response := postManualRecord(store, interval[0], interval[1])
		if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "时间冲突") {
			t.Fatalf("overlap %v accepted or lacks explanation: %d %s", interval, response.Code, response.Body.String())
		}
	}
	for _, interval := range [][2]string{{"08:00", "08:45"}, {"09:45", "10:00"}, {"11:00", "11:30"}} {
		response := postManualRecord(store, interval[0], interval[1])
		if response.Code != http.StatusOK {
			t.Fatalf("adjacent interval %v rejected: %d %s", interval, response.Code, response.Body.String())
		}
	}
}

func TestManualRecordRejectsShortBeforePersisting(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	response := postManualRecord(store, "09:00", "09:09")
	if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "不能少于 10 分钟") {
		t.Fatalf("short record accepted: %d %s", response.Code, response.Body.String())
	}
	response = postManualRecord(store, "09:00", "09:10")
	if response.Code != http.StatusOK {
		t.Fatalf("ten-minute record blocked: %d %s", response.Code, response.Body.String())
	}
	response = postManualRecord(store, "09:05", "09:15")
	if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), "时间冲突") {
		t.Fatalf("overlap accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestManualRecordChecksActiveSessionAndIgnoresDeleted(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	if _, err := testExec(store, `INSERT INTO sessions(subject_id,start_at,planned_end_at,source) VALUES(1,?,?,'timer')`,
		now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if response := postManualRecordAt(store, now.Add(-30*time.Minute), now.Add(-20*time.Minute)); response.Code == http.StatusOK {
		t.Fatal("active session overlap accepted")
	}
	if response := postManualRecordAt(store, now.Add(2*time.Hour), now.Add(3*time.Hour)); response.Code != http.StatusOK {
		t.Fatalf("future interval blocked by ongoing timer: %d %s", response.Code, response.Body.String())
	}
	if _, err := testExec(store, `UPDATE sessions SET deleted=1 WHERE source='timer'`); err != nil {
		t.Fatal(err)
	}
	if response := postManualRecordAt(store, now.Add(-30*time.Minute), now.Add(-20*time.Minute)); response.Code != http.StatusOK {
		t.Fatalf("deleted session still blocks: %d %s", response.Code, response.Body.String())
	}
}

func TestManualRecordAllowsTimeAfterPausedTimer(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	if _, err := testExec(store, `INSERT INTO sessions(subject_id,start_at,paused_at,source) VALUES(1,?,?,'timer')`, now.Add(-time.Hour).Format(time.RFC3339), now.Add(-30*time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if response := postManualRecordAt(store, now.Add(-20*time.Minute), now.Add(-10*time.Minute)); response.Code != http.StatusOK {
		t.Fatalf("paused timer blocked later interval: %d %s", response.Code, response.Body.String())
	}
}

func postManualRecord(store *App, start, end string) *httptest.ResponseRecorder {
	body := `{"subjectId":1,"startAt":"2026-11-04T` + start + `:00+08:00","endAt":"2026-11-04T` + end + `:00+08:00"}`
	response := httptest.NewRecorder()
	testRoute(store, store.sessions, response, httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(body)))
	return response
}

func postManualRecordAt(store *App, start, end time.Time) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"subjectId":1,"startAt":%q,"endAt":%q}`, start.Format(time.RFC3339), end.Format(time.RFC3339))
	response := httptest.NewRecorder()
	testRoute(store, store.sessions, response, httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(body)))
	return response
}
