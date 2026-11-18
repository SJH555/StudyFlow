package main

import (
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"net/http"

	"exam_notify/internal/modules/plans"
)

// Plan 是学习计划领域模型在桌面绑定层的别名。
type Plan = plans.Plan

func (a *App) plans(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := plans.List(db)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, items)
	case http.MethodPost, http.MethodPut:
		var item Plan
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			fail(w, err)
			return
		}
		save := plans.Create
		if r.Method == http.MethodPut {
			save = plans.Update
		}
		item, err := save(db, item)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, item)
	case http.MethodDelete:
		if err := plans.Delete(db, r.URL.Query().Get("id")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
