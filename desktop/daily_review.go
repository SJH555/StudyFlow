package main

import (
	"database/sql"
	"encoding/json"
	studyapp "exam_notify/internal/app"
	sound "exam_notify/internal/infrastructure/reminder"
	"exam_notify/internal/modules/review"
	"log"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type reviewItem = review.ReviewItem
type dailyReview = review.DailyReview

func (a *App) dailyReviewAPI(db *sql.DB, w http.ResponseWriter, r *http.Request, action string) {
	service := review.New(db)
	onSubmitted := func() {
		a.tasks.after(150*time.Millisecond, func() {
			a.releaseReviewGate(application.Get())
			a.releaseDeferredReminders(application.Get())
		})
	}
	switch {
	case action == "pending" && r.Method == http.MethodGet:
		pending, err := service.Pending(time.Now())
		if err != nil {
			fail(w, err)
			return
		}
		offer, err := service.BulkOffer(time.Now(), pending)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"pending": pending, "bulk": offer})
	case action == "history" && r.Method == http.MethodGet:
		items, err := service.History()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, items)
	case action == "submit" && r.Method == http.MethodPost:
		var input struct {
			Day     string       `json:"day"`
			Reasons []reviewItem `json:"reasons"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			fail(w, err)
			return
		}
		if err := service.Submit(input.Day, input.Reasons); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
		onSubmitted()
	case action == "bulk" && r.Method == http.MethodPost:
		var input struct {
			StartDay string `json:"startDay"`
			EndDay   string `json:"endDay"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			fail(w, err)
			return
		}
		count, err := service.SaveBulk(time.Now(), input.StartDay, input.EndDay, input.Reason)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "count": count})
		onSubmitted()
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type deferredReminder = studyapp.Reminder

func (a *App) deliverReminder(app *application.App, reminder deferredReminder) {
	if app == nil {
		return
	}
	switch a.gate.admit(reminder) {
	case admitDeliver:
		a.playReminder(app, reminder)
	case admitUpdate:
		emitReminderEvent(app, reminder)
	}
}

// playReminder 完整投递一条提醒：响铃、发前端事件与系统通知。
func (a *App) playReminder(app *application.App, reminder deferredReminder) {
	startReminderSound(reminder.Kind)
	emitReminderEvent(app, reminder)
	sound.Notify(reminder.Title, reminder.Body)
	wakeMainWindow(app)
}

// releaseNextReminder 当前弹窗关闭后立即投递排队中的下一条提醒。
func (a *App) releaseNextReminder(app *application.App) {
	if next := a.gate.release(); next != nil {
		a.playReminder(app, *next)
	}
}

// releaseReviewGate 每日复习完成后恢复门控：立即投递排队中的下一条提醒。
func (a *App) releaseReviewGate(app *application.App) {
	if next := a.gate.releaseReview(); next != nil {
		a.playReminder(app, *next)
	}
}

func startReminderSound(kind string) {
	switch kind {
	case "plan":
		sound.Start(sound.PlanSound)
	case "activity":
		sound.Start(sound.ActivitySound)
	case "break":
		sound.Start(sound.BreakSound)
	}
}

func emitReminderEvent(app *application.App, reminder deferredReminder) {
	switch reminder.Kind {
	case "plan":
		app.Event.Emit("plan-reminder", reminder.Payload)
	case "activity":
		app.Event.Emit("activity-reminder", reminder.Payload)
	case "break":
		app.Event.Emit("break-reminder", reminder.Payload)
	}
}

func (a *App) pendingReviewForReminder() (*dailyReview, error) {
	return a.scheduler.PendingReview(time.Now())
}

func (a *App) enqueueOrDeliver(desktop *application.App, reminder deferredReminder) {
	dispatch, err := a.reminders.Enqueue(reminder, a.pendingReviewForReminder)
	a.deliverDispatch(desktop, dispatch, err)
}

func (a *App) releaseDeferredReminders(desktop *application.App) {
	dispatch, err := a.reminders.Release(a.pendingReviewForReminder)
	a.deliverDispatch(desktop, dispatch, err)
}

func (a *App) deliverDispatch(desktop *application.App, dispatch studyapp.Dispatch, err error) {
	if err != nil {
		logReviewError(err)
		return
	}
	if desktop == nil {
		return
	}
	if dispatch.Review != nil {
		a.gate.holdReview()
		desktop.Event.Emit("daily-review-due", dispatch.Review.Day)
		wakeMainWindow(desktop)
	} else if dispatch.Reminder != nil {
		a.deliverReminder(desktop, *dispatch.Reminder)
	}
}

func logReviewError(err error) {
	if err != nil {
		log.Printf("检查昨日总结失败: %v", err)
	}
}
