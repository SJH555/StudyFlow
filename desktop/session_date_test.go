package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionsUseLocalDateForUTCManualRecords(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	localDate := time.Date(2026, 11, 4, 0, 45, 0, 0, time.Local)
	stored := localDate.UTC().Format(time.RFC3339)
	if _, err := testExec(store, `INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source) VALUES(1,?,?,3600,'manual')`, stored, localDate.Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	day := localDate.Format("2006-01-02")
	response := httptest.NewRecorder()
	testRoute(store, store.sessions, response, httptest.NewRequest(http.MethodGet, "/sessions?date="+day, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), stored) {
		t.Fatalf("local date %s did not include %s: %d %s", day, stored, response.Code, response.Body.String())
	}
}
