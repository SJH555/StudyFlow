package main

import (
	"context"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func breakReminderLoop(ctx context.Context, app *application.App, store *App) {
	for waitDesktopTask(ctx, time.Until(time.Now().Truncate(time.Second).Add(time.Second))) {
		subject, due, err := store.scheduler.BreakReminder(time.Now())
		if err != nil {
			log.Printf("检查休息提醒失败: %v", err)
			continue
		}
		if !due {
			continue
		}
		store.enqueueOrDeliver(app, deferredReminder{Kind: "break", Payload: subject, Title: "StudyFlow 休息结束", Body: subject + " · 可以继续学习了"})
	}
}
