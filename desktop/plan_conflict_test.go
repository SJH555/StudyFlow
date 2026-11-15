package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanConflictChecksWeekdaysTimeAndEdits(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	request := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		testRoute(store, store.plans, response, httptest.NewRequest(method, "/plans", strings.NewReader(body)))
		return response
	}
	base := request(http.MethodPost, `{"subjectId":1,"startTime":"09:00","duration":60,"weekdays":"1,3"}`)
	if base.Code != http.StatusOK {
		t.Fatal(base.Body.String())
	}
	for _, tc := range []struct {
		body     string
		conflict bool
	}{
		{`{"subjectId":1,"startTime":"09:30","duration":30,"weekdays":"2"}`, false},
		{`{"subjectId":1,"startTime":"10:00","duration":30,"weekdays":"1"}`, false},
		{`{"subjectId":1,"startTime":"09:30","duration":30,"weekdays":"1"}`, true},
	} {
		result := request(http.MethodPost, tc.body)
		if (result.Code != http.StatusOK) != tc.conflict {
			t.Fatalf("body=%s response=%d %s", tc.body, result.Code, result.Body.String())
		}
	}
	result := request(http.MethodPut, `{"id":1,"subjectId":1,"startTime":"09:00","duration":60,"weekdays":"1,3"}`)
	if result.Code != http.StatusOK {
		t.Fatalf("self edit: %d %s", result.Code, result.Body.String())
	}
	result = request(http.MethodPut, `{"id":2,"subjectId":1,"startTime":"09:30","duration":30,"weekdays":"1"}`)
	if result.Code == http.StatusOK {
		t.Fatal("conflicting edit accepted")
	}
}
