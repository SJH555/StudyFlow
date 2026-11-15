package main

import (
	"database/sql"
	"net/http"

	"exam_notify/internal/modules/settings"
)

func (a *App) resetTables(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tables, err := settings.ListResetTables(db)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, tables)
	case http.MethodDelete:
		if err := settings.ResetTable(db, r.URL.Query().Get("name")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
