package main

import (
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"net/http"
	"time"

	"exam_notify/internal/modules/sessions"
)

// Session 是学习记录领域模型在桌面绑定层的别名。
type Session = sessions.Session

func (a *App) sessions(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := sessions.List(db, r.URL.Query().Get("date"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, items)
	case http.MethodPost:
		var input sessions.ManualInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			fail(w, err)
			return
		}
		id, duration, err := sessions.AddManual(db, input)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"id": id, "duration": duration})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) sessionAction(db *persistence.Queries, w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPost:
		var input sessions.TimerInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			fail(w, err)
			return
		}
		now := time.Now()
		if input.Action == "start" {
			newID, err := sessions.Start(db, input, now)
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, map[string]any{"id": newID})
			return
		}
		saved, err := sessions.ChangeTimer(db, id, input.Action, now)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "saved": saved})
	case http.MethodDelete:
		if err := sessions.Delete(db, id); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) sessionDays(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := sessions.ListForDates(db, r.URL.Query()["date"])
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, items)
}
