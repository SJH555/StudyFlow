package main

import (
	"context"
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"exam_notify/internal/modules/review"
	"exam_notify/internal/modules/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log"
	"net/http"
	"time"
)

func (a *App) settings(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
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
		actions := settings.Actions{
			ActivityReminderChanged: a.onActivityReminderChanged,
			ReviewDisabled: func() {
				a.tasks.after(150*time.Millisecond, func() { a.releaseDeferredReminders(application.Get()) })
			},
			BackupEnabled: func() {
				a.tasks.start(func(ctx context.Context) {
					if ctx.Err() != nil {
						return
					}
					if err := a.scheduler.WithContext(ctx).BackupCheck(time.Now()); err != nil {
						log.Printf("自动备份失败: %v", err)
					}
				})
			},
			GeneralChanged: func() {
				a.tasks.start(func(ctx context.Context) {
					if ctx.Err() != nil {
						return
					}
					if err := a.applyGeneralSettingsContext(ctx); err != nil {
						log.Printf("刷新常规设置失败: %v", err)
					}
				})
			},
		}
		err := settings.Save(db, values, settings.Actions{
			ActivityReminderChanged: func(enabled bool) {
				if actions.ActivityReminderChanged != nil {
					queueAfterDatabase(r, func() { actions.ActivityReminderChanged(enabled) })
				}
			},
			ReviewDisabled: func() { queueAfterDatabase(r, actions.ReviewDisabled) },
			BeforeReviewRulesChange: func(tx *persistence.Queries, values map[string]string) error {
				return review.New(tx).SettleBeforeRuleChange(values, time.Now())
			},
			BackupEnabled:  func() { queueAfterDatabase(r, actions.BackupEnabled) },
			GeneralChanged: func() { queueAfterDatabase(r, actions.GeneralChanged) },
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
