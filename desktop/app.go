package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"

	studyapp "exam_notify/internal/app"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/reminder"
	"exam_notify/internal/modules/settings"
)

// App 聚合数据库、调度器与提醒队列，承载桌面应用的运行状态。
type App struct {
	database                  *persistence.Database
	scheduler                 *studyapp.Scheduler
	onActivityReminderChanged func(bool)
	reminders                 studyapp.ReminderQueue
	// gate 保证同一时刻只展示一个提醒弹窗（见 reminderGate）。
	gate reminderGate
	// generalMu 保护常规设置缓存：设置保存协程写入，主循环与窗口钩子读取。
	generalMu           sync.Mutex
	startupWindow       string // "show" 启动后显示主窗口，"tray" 启动后驻留托盘。
	closeBehavior       string // "tray" 关闭时隐藏到托盘，"quit" 关闭时退出应用。
	reviewCheckHour     int    // 进度追因检查的日界小时。
	reviewCheckMinute   int    // 进度追因检查的分钟。
	tasks               desktopTasks
	startupSchedule     *startupSchedule
	launcherJob         func(context.Context) error
	restartAfterRestore atomic.Bool
}

// applyGeneralSettings 从数据库读取常规设置并刷新缓存与铃声音量。
func (a *App) applyGeneralSettings() error {
	return a.applyGeneralSettingsContext(context.Background())
}

func (a *App) applyGeneralSettingsContext(ctx context.Context) error {
	var config settings.GeneralConfig
	var checkHour, checkMinute int
	err := a.database.WithRead(func(db *sql.DB, _ string) error {
		var readErr error
		config, readErr = settings.ReadGeneral(persistence.BindQueries(ctx, db))
		if readErr != nil {
			return readErr
		}
		checkHour, readErr = settings.CheckHour(persistence.BindQueries(ctx, db))
		if readErr != nil {
			return readErr
		}
		checkMinute, readErr = settings.CheckMinute(persistence.BindQueries(ctx, db))
		return readErr
	})
	if err != nil {
		return err
	}
	a.generalMu.Lock()
	a.startupWindow = config.StartupWindow
	a.closeBehavior = config.CloseBehavior
	a.reviewCheckHour = checkHour
	a.reviewCheckMinute = checkMinute
	a.generalMu.Unlock()
	reminder.SetVolume(config.RingtoneVolume)
	return nil
}

// startupShowsWindow 返回启动后是否直接显示主窗口。
func (a *App) startupShowsWindow() bool {
	a.generalMu.Lock()
	defer a.generalMu.Unlock()
	return a.startupWindow != "tray"
}

// quitOnClose 返回点击关闭按钮时是否退出应用。
func (a *App) quitOnClose() bool {
	a.generalMu.Lock()
	defer a.generalMu.Unlock()
	return a.closeBehavior == "quit"
}

// dailyCheckHour 返回进度追因检查的日界小时。
func (a *App) dailyCheckHour() int {
	a.generalMu.Lock()
	defer a.generalMu.Unlock()
	return a.reviewCheckHour
}

// dailyCheckMinute 返回进度追因检查的分钟。
func (a *App) dailyCheckMinute() int {
	a.generalMu.Lock()
	defer a.generalMu.Unlock()
	return a.reviewCheckMinute
}

func (a *App) activityReminderEnabled() (bool, error) {
	var enabled bool
	err := a.database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		enabled, err = settings.ActivityReminderEnabled(persistence.BindQueries(context.Background(), db))
		return err
	})
	return enabled, err
}

func (a *App) setActivityReminderEnabled(enabled bool) error {
	err := a.database.WithWrite(func(db *sql.DB, _ string) error {
		return settings.SetActivityReminderEnabled(persistence.BindQueries(context.Background(), db), enabled)
	})
	if err == nil && a.onActivityReminderChanged != nil {
		a.onActivityReminderChanged(enabled)
	}
	return err
}

func newApp(database *persistence.Database) *App {
	// 常规设置缓存先填默认值，runDesktop 启动时再用数据库中的设置覆盖。
	return &App{
		database:          database,
		startupSchedule:   newStartupSchedule(),
		scheduler:         studyapp.NewScheduler(database),
		startupWindow:     "show",
		closeBehavior:     "tray",
		reviewCheckHour:   6,
		reviewCheckMinute: 0,
	}
}

// Close 关闭应用持有的数据库连接。
func (a *App) Close() error {
	if a.database == nil {
		return nil
	}
	return a.database.Close()
}

// DataDirectory 返回数据库文件所在目录。
func (a *App) DataDirectory() string { return filepath.Dir(a.database.Path()) }
