package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanBreakMinutesRoundTripAndValidation(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRecorder()
	testRoute(store, store.plans, post, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"subjectId":1,"startTime":"09:00","duration":180,"breakMinutes":20,"weekdays":"1,2,3,4,5"}`)))
	if post.Code != http.StatusOK {
		t.Fatalf("create plan: %d %s", post.Code, post.Body.String())
	}
	list := httptest.NewRecorder()
	testRoute(store, store.plans, list, httptest.NewRequest(http.MethodGet, "/plans", nil))
	if !strings.Contains(list.Body.String(), `"breakMinutes":20`) {
		t.Fatalf("break duration missing from response: %s", list.Body.String())
	}
	invalid := httptest.NewRecorder()
	testRoute(store, store.plans, invalid, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"subjectId":1,"startTime":"11:00","duration":20,"breakMinutes":20,"weekdays":"1"}`)))
	if invalid.Code == http.StatusOK {
		t.Fatalf("break equal to plan duration accepted: %s", invalid.Body.String())
	}
}
