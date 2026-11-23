package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"exam_notify/internal/infrastructure/diagnostics"
	"exam_notify/internal/infrastructure/reminder"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	studyapp "exam_notify/internal/app"
	"exam_notify/internal/infrastructure/storage"
	"exam_notify/internal/infrastructure/system"
	"exam_notify/internal/shared/studycalendar"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:ui
var assets embed.FS

//go:embed assets/Icon.png
var appIcon []byte

//go:embed assets/app-icon.png
var trayIcon []byte

func init() {
	application.RegisterEvent[string]("activity-reminder")
	application.RegisterEvent[string]("activity-reminder-state")
	application.RegisterEvent[string]("break-reminder")
	application.RegisterEvent[string]("plan-reminder")
	application.RegisterEvent[string]("daily-review-due")
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	var restoredApp *App
	err := runWithStartupCheck(checkStartupTimeZone, func(store *App, instance system.InstanceGuard, base, data, location string, startup *startupUpgrade) error {
		restoredApp = store
		return runDesktopWithStartup(store, instance, base, data, location, startup, newDesktopTransport(), func(app *application.App) error { return app.Run() })
	})
	if err != nil || restoredApp == nil || !restoredApp.restartAfterRestore.Load() {
		return err
	}
	// 此时旧进程已关闭数据库和单实例锁，新进程可以按正常门禁初始化。
	executable := os.Getenv("APPIMAGE")
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	cmd := exec.Command(executable)
	cmd.Env = append(cmd.Environ(), "STUDYFLOW_REOPEN_AFTER_RESTORE=1")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("数据已恢复，请手动重新打开 StudyFlow: %w", err)
	}
	return cmd.Process.Release()
}

func runWithStartupPresenter(present func(*App, system.InstanceGuard, string, string, string, *startupUpgrade) error) error {
	return runWithStartupCheck(nil, present)
}

func runWithStartupCheck(check func() *startupStatus, present func(*App, system.InstanceGuard, string, string, string, *startupUpgrade) error) (resultErr error) {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	installDir, err := filepath.Abs(filepath.Dir(executable))
	if err != nil {
		return err
	}
	if appImage := os.Getenv("APPIMAGE"); appImage != "" {
		// AppImage 以只读方式挂载内部可执行文件，因此数据要存放在安装包旁边。
		installDir, err = filepath.Abs(filepath.Dir(appImage))
		if err != nil {
			return err
		}
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	// 保留旧的目录名，以便已有安装能够迁移其数据。
	configDir = filepath.Join(configDir, "StudyTime")
	homeDir := ""
	if runtime.GOOS == "linux" {
		var homeErr error
		homeDir, homeErr = os.UserHomeDir()
		if homeErr != nil {
			return homeErr
		}
	}
	baseDir := storage.BaseDir(runtime.GOOS, installDir, homeDir)
	instance, primary, err := system.AcquireSingleInstance(baseDir)
	if err != nil {
		return err
	}
	if !primary {
		return nil
	}
	defer func() { resultErr = errors.Join(resultErr, instance.Close()) }()
	defaultDataDir := filepath.Join(baseDir, "data")
	logDir := filepath.Join(baseDir, "logs")
	locationFile := filepath.Join(baseDir, "storage-location.txt")
	if filepath.Clean(logDir) != filepath.Clean(filepath.Join(installDir, "logs")) {
		if err = diagnostics.MigrateLegacyLogs(filepath.Join(installDir, "logs"), logDir); err != nil {
			return err
		}
	}
	if err = diagnostics.MigrateLegacyLogs(filepath.Join(configDir, "logs"), logDir); err != nil {
		return err
	}
	errorLog, err := diagnostics.StartErrorLog(logDir)
	if err != nil {
		return err
	}
	previousOutput := log.Writer()
	defer func() {
		if resultErr != nil {
			log.Print(resultErr)
		}
		log.SetOutput(previousOutput)
		appErrorLog = nil
		resultErr = errors.Join(resultErr, errorLog.Close())
	}()
	appErrorLog = errorLog
	log.SetOutput(io.MultiWriter(os.Stderr, errorLog))
	if check != nil {
		if blocked := check(); blocked != nil {
			app := newApp(nil)
			startup := &startupUpgrade{status: *blocked, tasks: &app.tasks}
			return present(app, instance, baseDir, defaultDataDir, locationFile, startup)
		}
	}
	checkingStarted := time.Now()
	store, startup := prepareStartupUpgrade(startupDatabasePaths{
		locationFile: locationFile, defaultDir: defaultDataDir, installDir: installDir, legacyConfigDir: configDir,
	})
	log.Printf("启动阶段 数据库准备 %s", time.Since(checkingStarted))
	store.launcherJob = func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return system.EnsureUserLauncher(executable, appIcon)
	}
	return present(store, instance, baseDir, defaultDataDir, locationFile, startup)
}

// 被接管的数据库连接属于本作用域，包括所有启动失败的情况。
func runDesktop(store *App, instance system.InstanceGuard, baseDir, defaultDataDir, locationFile string, transport application.Transport, runApp func(*application.App) error) error {
	return runDesktopWithStartup(store, instance, baseDir, defaultDataDir, locationFile, nil, transport, runApp)
}

func runDesktopWithStartup(store *App, instance system.InstanceGuard, baseDir, defaultDataDir, locationFile string, startup *startupUpgrade, transport application.Transport, runApp func(*application.App) error) (resultErr error) {
	var tracker *system.WindowStateTracker
	defer func() {
		store.tasks.stopAndWait()
		resultErr = errors.Join(resultErr, instance.StopListening())
		if tracker != nil {
			tracker.Close(nil)
		}
		reminder.Shutdown()
		resultErr = errors.Join(resultErr, transport.Stop(), store.Close())
	}()
	var initializeOnce sync.Once
	var initializeErr error
	var reminderEnabled bool
	initializeBusiness := func() error {
		initializeOnce.Do(func() {
			if err := os.MkdirAll(defaultDataDir, 0700); err != nil {
				initializeErr = err
				return
			}
			if err := reminder.Init(defaultDataDir); err != nil {
				initializeErr = err
				return
			}
			if err := store.applyGeneralSettings(); err != nil {
				initializeErr = err
				return
			}
			reminderEnabled, initializeErr = store.activityReminderEnabled()
		})
		return initializeErr
	}
	if startup.isReady() {
		if err := initializeBusiness(); err != nil {
			if startup == nil {
				return err
			}
			startup.fail(err)
			if closeErr := store.Close(); closeErr != nil {
				log.Print(closeErr)
			}
		}
	}
	if startup != nil {
		startup.activate = initializeBusiness
	}
	launch := &secondLaunch{tasks: &store.tasks}
	if err := instance.ListenForSecondLaunch(launch.notify); err != nil {
		return err
	}
	windowStatePath := filepath.Join(baseDir, "window-state.json")
	lastWindowState, stateErr := system.ReadWindowState(windowStatePath)
	if stateErr != nil {
		log.Printf("读取窗口位置和大小失败: %v", stateErr)
	}
	windowOptions := application.WebviewWindowOptions{
		Title:            "StudyFlow",
		Frameless:        runtime.GOOS == "windows",
		Width:            1380,
		Height:           920,
		MinWidth:         1000,
		MinHeight:        700,
		BackgroundColour: application.NewRGB(248, 248, 245),
		URL:              "/",
		// 启动后驻留托盘时先保持隐藏，启动完成后再按设置决定是否显示。
		Hidden: startup.isReady() && !store.startupShowsWindow() && os.Getenv("STUDYFLOW_REOPEN_AFTER_RESTORE") != "1",
	}
	system.ApplyWindowState(&windowOptions, lastWindowState, runtime.GOOS)
	var app *application.App
	var window *application.WebviewWindow
	app = application.New(application.Options{
		ShouldQuit: func() bool {
			return store.tasks.shouldQuit(func() {
				if err := instance.StopListening(); err != nil {
					log.Printf("停止第二实例监听失败: %v", err)
				}
				tracker.Close(window)
				reminder.Shutdown()
			}, func() { store.tasks.quitOnMainThread(application.InvokeAsync, app.Quit) })
		},
		Transport:   transport,
		Name:        "StudyFlow",
		Description: "学习时间管理与记录",
		Icon:        appIcon,
		Linux:       application.LinuxOptions{ApplicationID: "org.wails.studyflow", DisableQuitOnLastWindowClosed: true},
		Windows:     application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Services:    desktopServices(store, locationFile, startup),
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	window = app.Window.NewWithOptions(windowOptions)
	tracker = system.NewWindowStateTracker(windowStatePath, runtime.GOOS, lastWindowState)
	window.RegisterHook(events.Common.WindowDidMove, func(_ *application.WindowEvent) { tracker.Schedule(window) })
	window.RegisterHook(events.Common.WindowDidResize, func(_ *application.WindowEvent) { tracker.Schedule(window) })
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if !startup.isReady() {
			if !store.tasks.readyToQuit() {
				event.Cancel()
				app.Quit()
			}
			return
		}
		ctx, done, ok := store.tasks.enter()
		if !ok {
			if !store.tasks.readyToQuit() {
				event.Cancel()
			}
			return
		}
		defer done()
		if review, reviewErr := store.scheduler.WithContext(ctx).PendingReview(time.Now()); reviewErr != nil {
			log.Printf("检查待填总结失败: %v", reviewErr)
		} else if review != nil {
			event.Cancel()
			// 复习弹窗优先级最高：接管屏幕时停止铃声并让提醒排队。
			reminder.Stop()
			store.gate.holdReview()
			app.Event.Emit("daily-review-due", review.Day)
			window.Show()
			window.Focus()
			return
		}
		if store.quitOnClose() {
			// 关闭即退出：记录窗口状态后直接走应用的优雅退出流程。
			tracker.Schedule(window)
			app.Quit()
			return
		}
		tracker.Schedule(window)
		window.Hide()
		event.Cancel()
	})
	tray := app.SystemTray.New()
	if runtime.GOOS == "linux" {
		// Linux 托盘的悬浮提示取自 Label，SetTooltip 在当前 Wails 实现中不生效。
		tray.SetLabel("StudyFlow")
	}
	tray.SetIcon(trayIcon)
	tray.SetTooltip("StudyFlow")
	trayMenu := app.NewMenu()
	trayMenu.Add("打开 StudyFlow").OnClick(func(_ *application.Context) {
		_, done, ok := store.tasks.enter()
		if !ok {
			return
		}
		defer done()
		window.Show()
		window.Focus()
	})
	reminderLabel := func(enabled bool) string {
		if enabled {
			return "关闭活动提醒"
		}
		return "开启活动提醒"
	}
	reminderItem := trayMenu.Add(reminderLabel(reminderEnabled)).SetEnabled(startup.isReady())
	store.onActivityReminderChanged = func(enabled bool) {
		reminderItem.SetLabel(reminderLabel(enabled))
		value := "0"
		if enabled {
			value = "1"
		}
		app.Event.Emit("activity-reminder-state", value)
	}
	reminderItem.OnClick(func(_ *application.Context) {
		if !startup.isReady() {
			return
		}
		_, done, ok := store.tasks.enter()
		if !ok {
			return
		}
		defer done()
		enabled, readErr := store.activityReminderEnabled()
		if readErr != nil {
			log.Printf("读取活动提醒状态失败: %v", readErr)
			return
		}
		if toggleErr := store.setActivityReminderEnabled(!enabled); toggleErr != nil {
			log.Printf("切换活动提醒失败: %v", toggleErr)
		}
	})
	trayMenu.AddSeparator()
	trayMenu.Add("退出应用").OnClick(func(_ *application.Context) {
		app.Quit()
	})
	tray.SetMenu(trayMenu)
	tray.OnClick(func() {
		_, done, ok := store.tasks.enter()
		if !ok {
			return
		}
		defer done()
		window.Show()
		window.Focus()
	})
	var started sync.Once
	startBusiness := func() {
		if !startup.isReady() {
			return
		}
		started.Do(func() {
			reminderItem.SetLabel(reminderLabel(reminderEnabled))
			reminderItem.SetEnabled(true)
			store.tasks.start(func(ctx context.Context) { activityReminderLoop(ctx, app, store) })
			store.tasks.start(func(ctx context.Context) { breakReminderLoop(ctx, app, store) })
			startMaintenance(store)
		})
	}
	if startup != nil {
		startup.onReady = func() { application.InvokeAsync(startBusiness) }
	}
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		launch.activate(func() { window.Show(); window.Focus() })
		startBusiness()
	})
	runErr := runApp(app)
	if startup != nil {
		state := startup.snapshot()
		if state.Phase == "blocked" && state.Detail != "" {
			return errors.Join(runErr, errors.New(state.Detail))
		}
	}
	return runErr
}

func startMaintenance(store *App) {
	job := startupJob{
		name: "维护",
		run: func(ctx context.Context) error {
			if store.launcherJob != nil {
				if err := measureStartup("启动器", func() error { return store.launcherJob(ctx) }); err != nil {
					log.Printf("创建用户快捷方式失败: %v", err)
				}
			}
			store.tasks.start(func(ctx context.Context) { autoBackupLoop(ctx, store) })
			return nil
		},
	}
	store.tasks.start(func(ctx context.Context) {
		store.startupSchedule.runAfterHome(ctx, job)
	})
}

func activityReminderLoop(ctx context.Context, app *application.App, store *App) {
	lastPlanMinute := ""
	lastActivityMinute := ""
	// 将检查对齐到每秒的起始，使提醒在整秒（:00）触发。
	for waitDesktopTask(ctx, time.Until(time.Now().Truncate(time.Second).Add(time.Second))) {
		now := time.Now().In(studycalendar.Location)
		if now.Hour() == 0 && now.Minute() == 0 && now.Second() == 0 {
			if err := store.scheduler.WithContext(ctx).DailySnapshot(now); err != nil {
				log.Printf("%v", err)
			}
		}
		minuteKey := now.Format("2006-01-02 15:04")
		checkStart := now.Second() == 0 && minuteKey != lastPlanMinute
		if checkStart {
			lastPlanMinute = minuteKey
		}
		alert, err := store.scheduler.WithContext(ctx).PlanReminder(now, checkStart)
		if err != nil {
			log.Printf("%v", err)
		}
		if alert != nil {
			payload, _ := json.Marshal(alert)
			store.enqueueOrDeliver(app, deferredReminder{Kind: "plan", Payload: string(payload), Title: "StudyFlow 学习计划", Body: studyapp.PlanAlertText(*alert)})
		}
		if now.Second() != 0 {
			continue
		}
		if now.Hour() == store.dailyCheckHour() && now.Minute() == store.dailyCheckMinute() {
			if review, reviewErr := store.scheduler.WithContext(ctx).PendingReview(now); reviewErr != nil {
				log.Printf("检查昨日总结失败: %v", reviewErr)
			} else if review != nil {
				reminder.Stop()
				store.gate.holdReview()
				app.Event.Emit("daily-review-due", review.Day)
				wakeMainWindow(app)
			}
		}
		fire, activityErr := store.scheduler.WithContext(ctx).ActivityReminder(now)
		if activityErr != nil {
			log.Printf("检查活动提醒失败: %v", activityErr)
		} else if fire && minuteKey != lastActivityMinute {
			lastActivityMinute = minuteKey
			store.enqueueOrDeliver(app, deferredReminder{Kind: "activity", Payload: now.Format(time.RFC3339), Title: "StudyFlow 活动提醒", Body: "请填写现在正在做的事"})
		}
	}
}

func wakeMainWindow(app *application.App) {
	for _, window := range app.Window.GetAll() {
		if window.IsMinimised() {
			window.Restore()
		}
		window.Hide()
		window.Show()
		window.SetAlwaysOnTop(true)
		window.Focus()
	}
}

func desktopServices(store *App, locationFile string, startup *startupUpgrade) []application.Service {
	services := []application.Service{application.NewService(&APIService{app: store, locationFile: locationFile, startup: startup})}
	if startup == nil {
		startup = &startupUpgrade{status: startupStatus{Phase: "ready"}}
	}
	return append(services, application.NewService(&StartupService{control: startup}))
}
