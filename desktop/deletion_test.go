package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeletedPlanAndSessionStayInDatabaseButDisappearFromLists(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-04')`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(store, `INSERT INTO plans(id,subject_id,start_time,duration_minutes,weekdays) VALUES(7,1,'09:00',60,'0,1,2,3,4,5,6')`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(store, `INSERT INTO sessions(id,subject_id,plan_id,start_at,end_at,duration_seconds) VALUES(9,1,7,'2026-11-04T09:00:00+08:00','2026-11-04T10:00:00+08:00',3600)`); err != nil {
		t.Fatal(err)
	}
	planDelete := httptest.NewRecorder()
	testRoute(store, store.plans, planDelete, httptest.NewRequest(http.MethodDelete, "/plans?id=7", nil))
	if planDelete.Code != http.StatusOK {
		t.Fatalf("plan delete: %d %s", planDelete.Code, planDelete.Body.String())
	}
	sessionDelete := httptest.NewRecorder()
	testSessionAction(store, sessionDelete, httptest.NewRequest(http.MethodDelete, "/sessions/9", nil), "9")
	if sessionDelete.Code != http.StatusOK {
		t.Fatalf("session delete: %d %s", sessionDelete.Code, sessionDelete.Body.String())
	}
	for table, id := range map[string]int{"plans": 7, "sessions": 9} {
		var deleted int
		if err := testQueryRow(store, `SELECT deleted FROM `+table+` WHERE id=?`, id).Scan(&deleted); err != nil || deleted != 1 {
			t.Fatalf("%s row missing or not logically deleted: %d %v", table, deleted, err)
		}
	}
	plans := httptest.NewRecorder()
	testRoute(store, store.plans, plans, httptest.NewRequest(http.MethodGet, "/plans", nil))
	if strings.Contains(plans.Body.String(), `"id":7`) {
		t.Fatalf("deleted plan visible: %s", plans.Body.String())
	}
	sessions := httptest.NewRecorder()
	testRoute(store, store.sessions, sessions, httptest.NewRequest(http.MethodGet, "/sessions?date=2026-11-04", nil))
	if strings.Contains(sessions.Body.String(), `"id":9`) {
		t.Fatalf("deleted session visible: %s", sessions.Body.String())
	}
}
