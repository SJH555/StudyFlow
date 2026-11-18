package main

import (
	"encoding/json"
	"errors"
	studyapp "exam_notify/internal/app"
	"exam_notify/internal/infrastructure/diagnostics"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/reminder"
	"exam_notify/internal/modules/review"
	"exam_notify/internal/modules/subjects"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var appErrorLog *diagnostics.Logger

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	log.Printf("API error: %v", err)
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func apiRoute(r *http.Request) string {
	return strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
}

func (s *APIService) handleDatabaseFreeAPI(path string, w http.ResponseWriter, r *http.Request) bool {
	if path == "reminder/ack" && r.Method == http.MethodPost {
		s.app.acknowledgeReminder(w, reminder.Stop, clearReminderAlwaysOnTop, s.afterReminderAck)
		return true
	}
	if path == "logs" && r.Method == http.MethodPost {
		var in struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			fail(w, err)
			return true
		}
		message := strings.TrimSpace(in.Message)
		if len(message) > 2000 {
			message = message[:2000]
		}
		if message != "" {
			log.Printf("UI error: %s", message)
		}
		writeJSON(w, map[string]bool{"ok": true})
		return true
	}
	if path == "logs" && r.Method == http.MethodGet {
		if appErrorLog == nil {
			fail(w, errors.New("error log unavailable"))
			return true
		}
		if name := r.URL.Query().Get("name"); name != "" {
			content, err := appErrorLog.Read(name)
			if err != nil {
				fail(w, err)
				return true
			}
			writeJSON(w, map[string]string{"content": content})
			return true
		}
		files, err := appErrorLog.List()
		if err != nil {
			fail(w, err)
			return true
		}
		writeJSON(w, map[string]any{"directory": appErrorLog.Directory(), "retentionDays": 7, "files": files})
		return true
	}
	return false
}

func (a *App) acknowledgeReminder(w http.ResponseWriter, stop, clearTop func(), after func(time.Duration, func())) {
	stop()
	a.reminders.Acknowledge()
	clearTop()
	writeJSON(w, map[string]bool{"ok": true})

	release := func() {
		a.releaseDeferredReminders(application.Get())
		a.releaseNextReminder(application.Get())
	}
	if after != nil {
		after(150*time.Millisecond, release)
		return
	}
	a.tasks.after(150*time.Millisecond, release)
}

func clearReminderAlwaysOnTop() {
	if desktop := application.Get(); desktop != nil {
		for _, window := range desktop.Window.GetAll() {
			window.SetAlwaysOnTop(false)
		}
	}
}

func (a *App) handleScopedAPI(access *persistence.Access, w http.ResponseWriter, r *http.Request) {
	// Every module receives context-bound queries and the same transaction scope.
	db := access.Queries(r.Context())
	a.handleAPI(db, w, r)
}

func (a *App) handleAPI(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	path := apiRoute(r)
	p := strings.Split(path, "/")
	if path == "snapshot" && r.Method == http.MethodGet {
		a.snapshot(db, w, r)
		return
	}
	if path == "subjects" {
		a.withScheduleSnapshot(db, w, r, a.subjects)
		return
	}
	if strings.HasPrefix(path, "daily-review/") {
		a.dailyReviewAPI(db, w, r, strings.TrimPrefix(path, "daily-review/"))
		return
	}
	if path == "goals" || strings.HasPrefix(path, "goals/") {
		a.goalsAPI(db, w, r, strings.TrimPrefix(path, "goals"))
		return
	}
	if path == "reset-tables" {
		a.resetTables(db, w, r)
		return
	}
	if path == "plans" {
		a.withScheduleSnapshot(db, w, r, a.plans)
		return
	}
	if path == "sessions" {
		a.sessions(db, w, r)
		return
	}
	if len(p) == 2 && p[0] == "sessions" {
		a.sessionAction(db, w, r, p[1])
		return
	}
	if path == "activities" {
		a.activities(db, w, r)
		return
	}
	if path == "settings" {
		a.settings(db, w, r)
		return
	}
	http.NotFound(w, r)
}

func (a *App) withScheduleSnapshot(db *persistence.Queries, w http.ResponseWriter, r *http.Request, handle func(*persistence.Queries, http.ResponseWriter, *http.Request)) {
	if r.Method == http.MethodGet {
		handle(db, w, r)
		return
	}
	if err := studyapp.ApplyScheduleChange(review.New(db), time.Now(), func() { handle(db, w, r) }); err != nil {
		fail(w, err)
	}
}

func (a *App) snapshot(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	count, e := subjects.Count(db)
	if e != nil {
		fail(w, e)
		return
	}
	writeJSON(w, map[string]any{"date": date, "now": time.Now().Format(time.RFC3339), "subjectsCount": count})
}
