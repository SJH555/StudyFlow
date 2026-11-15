package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingDatabaseAddsNoteAndSortColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL, created_at TEXT NOT NULL, archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1, color TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0, break_minutes INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-01')`,
		`INSERT INTO plans(id,subject_id,start_time,duration_minutes,weekdays) VALUES(1,1,'09:00',60,'1,2,3,4,5')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := NewApp(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var sort sql.NullInt64
	var note string
	if err := testQueryRow(store, `SELECT sort_order FROM subjects WHERE id=1`).Scan(&sort); err != nil || sort.Valid {
		t.Fatalf("sort migration: %v %v", sort, err)
	}
	if err := testQueryRow(store, `SELECT note FROM plans WHERE id=1`).Scan(&note); err != nil || note != "" {
		t.Fatalf("note migration: %q %v", note, err)
	}
}

func TestSubjectCustomSortThenUnsortedCreationOrder(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, subject := range []struct {
		name, created string
		order         any
	}{
		{"甲", "2026-11-01T09:00:00+08:00", nil},
		{"乙", "2026-11-02T09:00:00+08:00", nil},
		{"丙", "2026-11-03T09:00:00+08:00", 2},
		{"丁", "2026-11-04T09:00:00+08:00", 1},
	} {
		if _, err := testExec(store, `INSERT INTO subjects(name,color,created_at,sort_order) VALUES(?,'#007AFF',?,?)`, subject.name, subject.created, subject.order); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	testRoute(store, store.subjects, response, httptest.NewRequest(http.MethodGet, "/subjects", nil))
	var got []Subject
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("returned %d subjects: %s", len(got), response.Body.String())
	}
	for index, name := range []string{"丁", "丙", "甲", "乙"} {
		if got[index].Name != name {
			t.Fatalf("position %d: got %q, want %q", index, got[index].Name, name)
		}
	}
}

func TestSubjectSortOrderCanBeSetAndCleared(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	created := httptest.NewRecorder()
	testRoute(store, store.subjects, created, httptest.NewRequest(http.MethodPost, "/subjects", strings.NewReader(`{"name":"数学","color":"#007AFF","sortOrder":3}`)))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"sortOrder":3`) {
		t.Fatalf("create with sort order: %d %s", created.Code, created.Body.String())
	}
	cleared := httptest.NewRecorder()
	testRoute(store, store.subjects, cleared, httptest.NewRequest(http.MethodPut, "/subjects", strings.NewReader(`{"id":1,"name":"数学","color":"#007AFF","sortOrder":null}`)))
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear sort order: %d %s", cleared.Code, cleared.Body.String())
	}
	var sort sql.NullInt64
	if err := testQueryRow(store, `SELECT sort_order FROM subjects WHERE id=1`).Scan(&sort); err != nil || sort.Valid {
		t.Fatalf("sort order not cleared: %v %v", sort, err)
	}
}

func TestDeletedPlanNoLongerBlocksSubjectDeletionButVisibleSessionDoes(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := testExec(store, `INSERT INTO plans(subject_id,start_time,duration_minutes,deleted) VALUES(1,'09:00',60,1)`); err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	testRoute(store, store.subjects, list, httptest.NewRequest(http.MethodGet, "/subjects", nil))
	var got []Subject
	if err := json.Unmarshal(list.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0].HasData {
		t.Fatalf("deleted plan still blocks subject: %+v %v", got, err)
	}
	if _, err := testExec(store, `INSERT INTO sessions(subject_id,start_at,end_at) VALUES(1,'2026-11-01T09:00:00+08:00','2026-11-01T10:00:00+08:00')`); err != nil {
		t.Fatal(err)
	}
	blocked := httptest.NewRecorder()
	testRoute(store, store.subjects, blocked, httptest.NewRequest(http.MethodDelete, "/subjects?id=1", nil))
	if blocked.Code == http.StatusOK {
		t.Fatal("visible study record did not block subject deletion")
	}
	if _, err := testExec(store, `UPDATE sessions SET deleted=1 WHERE subject_id=1`); err != nil {
		t.Fatal(err)
	}
	deleted := httptest.NewRecorder()
	testRoute(store, store.subjects, deleted, httptest.NewRequest(http.MethodDelete, "/subjects?id=1", nil))
	if deleted.Code != http.StatusOK {
		t.Fatalf("subject deletion failed: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestPlanNoteRoundTripAndLimit(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := testExec(store, `INSERT INTO subjects(id,name,color,created_at) VALUES(1,'数学','#007AFF','2026-11-01')`); err != nil {
		t.Fatal(err)
	}
	created := httptest.NewRecorder()
	testRoute(store, store.plans, created, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"subjectId":1,"startTime":"09:00","duration":60,"weekdays":"1,2,3,4,5","note":"第 3 章导数定义与例题"}`)))
	if created.Code != http.StatusOK {
		t.Fatalf("create plan: %d %s", created.Code, created.Body.String())
	}
	listed := httptest.NewRecorder()
	testRoute(store, store.plans, listed, httptest.NewRequest(http.MethodGet, "/plans", nil))
	if !strings.Contains(listed.Body.String(), `"note":"第 3 章导数定义与例题"`) {
		t.Fatalf("note missing: %s", listed.Body.String())
	}
	tooLong := httptest.NewRecorder()
	testRoute(store, store.plans, tooLong, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"subjectId":1,"startTime":"11:00","duration":60,"weekdays":"1,2,3,4,5","note":"`+strings.Repeat("学", 101)+`"}`)))
	if tooLong.Code == http.StatusOK {
		t.Fatal("101-character note was accepted")
	}
}
