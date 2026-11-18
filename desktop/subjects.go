package main

import (
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"net/http"

	"exam_notify/internal/modules/subjects"
)

// Subject 是学习科目领域模型在桌面绑定层的别名。
type Subject = subjects.Subject

func (a *App) subjects(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		items, err := subjects.List(db, q.Get("includeArchived") == "true", q.Get("includeDeleted") == "true")
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, items)
	case http.MethodPost, http.MethodPut:
		var item Subject
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			fail(w, err)
			return
		}
		save := subjects.Create
		if r.Method == http.MethodPut {
			save = subjects.Update
		}
		item, err := save(db, item)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, item)
	case http.MethodDelete:
		if err := subjects.Delete(db, r.URL.Query().Get("id")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
