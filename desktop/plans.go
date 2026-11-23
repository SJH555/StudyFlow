package main

import (
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"

	"exam_notify/internal/modules/plans"
)

// Plan 是学习计划领域模型在桌面绑定层的别名。
type Plan = plans.Plan

func (a *App) plans(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		fail(w, err)
		return
	}
	if history {
		a.versionedPlans(db, w, r, time.Now())
		return
	}
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

func (a *App) versionedPlans(db *persistence.Queries, w http.ResponseWriter, r *http.Request, now time.Time) {
	query := r.URL.Query()
	if r.Method == http.MethodGet {
		var result any
		var err error
		switch {
		case query.Has("date"):
			result, err = plans.DailyPlans(db, query["date"], now)
		case query.Has("versions"):
			var id int64
			id, err = strconv.ParseInt(query.Get("versions"), 10, 64)
			if err == nil {
				result, err = plans.HistoryVersions(db, id)
			}
		case query.Get("management") == "true":
			result, err = plans.Management(db, now)
		default:
			result, err = plans.AsOf(db, studycalendar.Day(now))
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, result)
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if !confirmEffectiveDay(w, r, now) {
			return
		}
		var item Plan
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			fail(w, err)
			return
		}
		saved, err := plans.SaveVersion(db, item, r.Method == http.MethodPost, now)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, saved)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !confirmEffectiveDay(w, r, now) {
		return
	}
	id, err := strconv.ParseInt(query.Get("id"), 10, 64)
	if err != nil {
		fail(w, err)
		return
	}
	status := plans.Deleted
	if r.Method == http.MethodPatch {
		status = query.Get("status")
	}
	if err := plans.ChangeStatus(db, id, status, now); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"effectiveDay": studycalendar.Tomorrow(now)})
}
