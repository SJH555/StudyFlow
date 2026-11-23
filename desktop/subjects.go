package main

import (
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"
	"net/http"
	"time"

	"exam_notify/internal/modules/subjects"
)

// Subject 是学习科目领域模型在桌面绑定层的别名。
type Subject = subjects.Subject

func (a *App) subjects(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		fail(w, err)
		return
	}
	now := time.Now()
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		includeStopped, includeDeleted := q.Get("includeArchived") == "true", q.Get("includeDeleted") == "true"
		var items []Subject
		var err error
		if day := q.Get("manualDate"); day != "" {
			if _, err := time.Parse(time.DateOnly, day); err != nil {
				fail(w, err)
				return
			}
			items, err = subjects.ManualOptions(db, day)
		} else if history {
			day := studycalendar.Day(now)
			if q.Get("management") == "true" {
				day = studycalendar.Tomorrow(now)
			}
			items, err = subjects.AsOf(db, day, includeStopped, includeDeleted)
		} else {
			items, err = subjects.List(db, includeStopped, includeDeleted)
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, items)
	case http.MethodPost, http.MethodPut:
		if history && r.Method == http.MethodPut && !confirmEffectiveDay(w, r, now) {
			return
		}
		var item Subject
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			fail(w, err)
			return
		}
		save := subjects.Create
		if r.Method == http.MethodPut {
			save = subjects.Update
		}
		var err error
		if history {
			item, err = subjects.SaveVersion(db, item, r.Method == http.MethodPost, now)
		} else {
			item, err = save(db, item)
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, item)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		var err error
		if history {
			if !confirmEffectiveDay(w, r, now) {
				return
			}
			err = subjects.DeleteVersion(db, id, now)
		} else {
			err = subjects.Delete(db, id)
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
