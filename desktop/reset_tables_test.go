package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestResetTablesBindingPreservesAllowlistAndLinkedDataProtection(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, query := range []string{
		"CREATE TABLE outside_allowlist (id INTEGER)",
		"INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-04')",
		"INSERT INTO plans(subject_id,start_time,duration_minutes,deleted) VALUES(1,'09:00',30,1)",
		"INSERT INTO sessions(subject_id,start_at,deleted) VALUES(1,'2026-11-04T09:00:00+08:00',1)",
	} {
		if _, err := testExec(store, query); err != nil {
			t.Fatal(err)
		}
	}

	api := &APIService{app: store}
	var tables []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(api.Call("reset-tables", http.MethodGet, "")), &tables); err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, table := range tables {
		counts[table.Name] = table.Count
		if table.Name == "subjects" && table.Label != "学习项目" {
			t.Fatalf("unexpected subject label: %q", table.Label)
		}
	}
	if _, listed := counts["outside_allowlist"]; listed || counts["subjects"] != 1 || counts["plans"] != 1 || counts["sessions"] != 1 {
		t.Fatalf("unexpected reset table counts: %v", counts)
	}

	reset := func(name, want string) {
		t.Helper()
		if got := api.Call("reset-tables?name="+name, http.MethodDelete, ""); got != want {
			t.Fatalf("reset %s: got %s, want %s", name, got, want)
		}
	}
	const linkedError = `{"error":"请先重置学习记录和学习计划，再重置学习项目"}`
	const ok = "{\"ok\":true}\n"
	reset("outside_allowlist", `{"error":"不支持重置此表"}`)
	reset("subjects", linkedError)
	reset("plans", ok)
	reset("subjects", linkedError)
	reset("sessions", ok)
	reset("subjects", ok)
}
