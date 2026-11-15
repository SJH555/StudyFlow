package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"exam_notify/internal/infrastructure/reminder"
	"exam_notify/internal/modules/activities"
)

func (a *App) activities(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := activities.List(db, r.URL.Query().Get("date"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, items)
	case http.MethodPost:
		var input activities.Input
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			fail(w, err)
			return
		}
		id, err := activities.Add(db, input)
		if err != nil {
			fail(w, err)
			return
		}
		reminder.StopIfKind(reminder.ActivitySound)
		writeJSON(w, map[string]any{"id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
