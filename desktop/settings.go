package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"exam_notify/internal/modules/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log"
	"net/http"
	"time"
)

func (a *App) settings(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		values, err := settings.List(db)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, values)
	case http.MethodPut:
		var values map[string]string
		if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
			fail(w, err)
			return
		}
		err := settings.Save(db, values, settings.Actions{
			ActivityReminderChanged: a.onActivityReminderChanged,
			ReviewDisabled: func() {
				a.tasks.after(150*time.Millisecond, func() { a.releaseDeferredReminders(application.Get()) })
			},
			BackupEnabled: func() {
				a.tasks.start(func(ctx context.Context) {
					if ctx.Err() != nil {
						return
					}
					if err := a.scheduler.BackupCheck(time.Now()); err != nil {
						log.Printf("自动备份失败: %v", err)
					}
				})
			},
			GeneralChanged: func() {
				a.tasks.start(func(ctx context.Context) {
					if ctx.Err() != nil {
						return
					}
					if err := a.applyGeneralSettings(); err != nil {
						log.Printf("刷新常规设置失败: %v", err)
					}
				})
			},
		})
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
