package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/sessions"
	"exam_notify/internal/modules/subjects"
	"exam_notify/internal/shared/studycalendar"
)

func TestScheduleHistoryKeepsTodayAndRunningBreakAfterRestart(t *testing.T) {
	paths, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now().In(studycalendar.Location)
	firstDay := time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, now.Location())
	planDay := firstDay.AddDate(0, 0, 1)
	var planID, planVersion, sessionID int64
	err := app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		db := a.Queries(context.Background())
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "数学"}, true, firstDay)
		if err != nil {
			return err
		}
		plan := plans.Plan{SubjectID: subject.ID, StartTime: "11:00", Duration: 120, BreakMinutes: 20, Weekdays: "0,1,2,3,4,5,6"}
		plan, err = plans.SaveVersion(db, plan, true, firstDay)
		if err != nil {
			return err
		}
		planID, planVersion = plan.ID, plan.VersionID
		manualStart := firstDay.Add(-30 * time.Minute)
		if _, _, err := sessions.AddManual(db, sessions.ManualInput{
			SubjectID: subject.ID,
			StartAt:   manualStart.Format(time.RFC3339),
			EndAt:     firstDay.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		sessionID, err = sessions.Start(db, sessions.TimerInput{PlanID: planID, SubjectID: subject.ID}, planDay)
		if err != nil {
			return err
		}
		plan.BreakMinutes = 10
		plan.Duration = 90
		if _, err := plans.SaveVersion(db, plan, false, planDay); err != nil {
			return err
		}
		plan.BreakMinutes = 5
		plan.Duration = 60
		if _, err := plans.SaveVersion(db, plan, false, planDay); err != nil {
			return err
		}
		subject.Name = "高等数学"
		if _, err := subjects.SaveVersion(db, subject, false, planDay); err != nil {
			return err
		}
		_, err = sessions.ChangeTimer(db, strconv.FormatInt(sessionID, 10), "pause", planDay.Add(20*time.Minute))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, startup := prepareStartupUpgrade(paths)
	defer reopened.Close()
	if !startup.isReady() {
		t.Fatal(startup.snapshot())
	}
	err = reopened.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		db := a.Queries(context.Background())
		today, tomorrow := studycalendar.Day(planDay), studycalendar.Tomorrow(planDay)
		items, err := plans.DailyPlans(db, []string{today, tomorrow}, planDay)
		if err != nil {
			return err
		}
		before, after := items[today].Plans, items[tomorrow].Plans
		if len(before) != 1 || len(after) != 1 || before[0].VersionID != planVersion ||
			before[0].Duration != 120 || before[0].BreakMinutes != 20 ||
			after[0].Duration != 60 || after[0].BreakMinutes != 5 ||
			before[0].Subject != "数学" || after[0].Subject != "高等数学" {
			t.Fatalf("历史版本不符合生效日: %+v", items)
		}
		manual, err := sessions.List(db, studycalendar.Day(firstDay))
		if err != nil {
			return err
		}
		if len(manual) != 1 || manual[0].Subject != "数学" || manual[0].Duration != 1800 {
			t.Fatalf("手动补录没有保留历史项目和时长: %+v", manual)
		}
		rows, err := sessions.List(db, today)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].ID != sessionID || rows[0].PlanVersionID == nil ||
			*rows[0].PlanVersionID != planVersion || rows[0].BreakMinutes != 20 || rows[0].BreakStartedAt == nil {
			t.Fatalf("没有恢复原休息状态: %+v", rows)
		}
		for _, minutes := range []int{26, 40, 41} {
			_, due, err := sessions.DueBreakReminder(db, planDay.Add(time.Duration(minutes)*time.Minute))
			if err != nil {
				return err
			}
			if due != (minutes == 40) {
				t.Fatalf("休息提醒与会话原版本不一致，分钟=%d，触发=%t", minutes, due)
			}
		}
		if err := plans.ChangeStatus(db, planID, plans.Stopped, planDay); err != nil {
			return err
		}
		for _, offset := range []int{0, 1} {
			managed, err := plans.Management(db, planDay.AddDate(0, 0, offset))
			if err != nil {
				return err
			}
			if len(managed) != 1 || managed[0].ManagedActive != (offset == 0) ||
				managed[0].Pending != (offset == 0) || managed[0].PendingStatus != (offset == 0) {
				t.Fatalf("计划停用前后应分别出现在循环和停用列表: %+v", managed)
			}
		}
		versions, err := plans.HistoryVersions(db, planID)
		if err != nil {
			return err
		}
		if len(versions) != 4 || versions[0].Status != plans.Stopped ||
			versions[3].VersionID != planVersion {
			t.Fatalf("计划版本记录未返回所有版本: %+v", versions)
		}
		if err := plans.ChangeStatus(db, planID, plans.Deleted, planDay); err != nil {
			return err
		}
		items, err = plans.DailyPlans(db, []string{today, tomorrow}, planDay)
		if err != nil {
			return err
		}
		if len(items[today].Plans) != 1 || len(items[tomorrow].Plans) != 0 {
			t.Fatalf("逻辑删除污染历史: %+v", items)
		}
		managed, err := plans.Management(db, planDay)
		if err != nil {
			return err
		}
		if len(managed) != 1 || managed[0].VersionID != planVersion || managed[0].Status != plans.Deleted ||
			!managed[0].ManagedActive || managed[0].PendingLabel != "明日起删除" || managed[0].PendingDay != tomorrow {
			t.Fatalf("待删除计划应保留在原列表并显示今日版本: %+v", managed)
		}
		managed, err = plans.Management(db, planDay.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		if len(managed) != 0 {
			t.Fatalf("删除生效后计划应移出列表: %+v", managed)
		}
		recreated, err := plans.SaveVersion(db, before[0], true, planDay)
		if err != nil {
			return err
		}
		items, err = plans.DailyPlans(db, []string{today, tomorrow}, planDay)
		if err != nil {
			return err
		}
		if len(items[tomorrow].Plans) != 1 || items[tomorrow].Plans[0].ID == planID ||
			items[tomorrow].Plans[0].ID != recreated.ID {
			t.Fatalf("同名重建没有保留独立计划身份: %+v", items)
		}
		if err := subjects.DeleteVersion(db, strconv.FormatInt(before[0].SubjectID, 10), planDay); err != nil {
			return err
		}
		items, err = plans.DailyPlans(db, []string{today, tomorrow}, planDay)
		if err != nil {
			return err
		}
		if len(items[today].Plans) != 1 || len(items[tomorrow].Plans) != 0 {
			t.Fatalf("删除科目未分别处理历史和未来的状态: %+v", items)
		}
		rows, err = sessions.List(db, today)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].PlanID == nil || *rows[0].PlanID != planID || rows[0].Subject != "数学" {
			t.Fatalf("删除和重建改变了原学习记录: %+v", rows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScheduleHistoryCancelsPendingStopAndReenablesNextDay(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	defer app.Close()
	now := time.Now().In(studycalendar.Location)
	day := now.AddDate(0, 0, 1)
	err := app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		db := a.Queries(context.Background())
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "数学"}, true, now)
		if err != nil {
			return err
		}
		original, err := plans.SaveVersion(db, plans.Plan{
			SubjectID: subject.ID, StartTime: "11:00", Duration: 60,
			BreakMinutes: 10, Weekdays: "0,1,2,3,4,5,6",
		}, true, now)
		if err != nil {
			return err
		}
		for _, edited := range []bool{false, true} {
			if edited {
				edit := original
				edit.BreakMinutes = 5
				if _, err := plans.SaveVersion(db, edit, false, day); err != nil {
					return err
				}
			}
			if err := plans.ChangeStatus(db, original.ID, plans.Stopped, day); err != nil {
				return err
			}
			if err := plans.ChangeStatus(db, original.ID, plans.Active, day); err != nil {
				return err
			}
			managed, err := plans.Management(db, day)
			if err != nil {
				return err
			}
			if len(managed) != 1 || !managed[0].ManagedActive || !managed[0].Active ||
				managed[0].PendingStatus || managed[0].Pending != edited || edited && managed[0].PendingLabel != "明日起变更" {
				t.Fatalf("撤销停用应立即恢复循环状态，已有修改仍待次日生效: %+v", managed)
			}
			today, err := plans.AsOf(db, studycalendar.Day(day))
			if err != nil {
				return err
			}
			if len(today) != 1 || today[0].VersionID != original.VersionID {
				t.Fatalf("撤销停用改动了当天版本: %+v", today)
			}
		}

		if err := plans.ChangeStatus(db, original.ID, plans.Stopped, day); err != nil {
			return err
		}
		stoppedDay := day.AddDate(0, 0, 1)
		if err := plans.ChangeStatus(db, original.ID, plans.Active, stoppedDay); err != nil {
			return err
		}
		today, err := plans.AsOf(db, studycalendar.Day(stoppedDay))
		if err != nil {
			return err
		}
		tomorrow, err := plans.AsOf(db, studycalendar.Tomorrow(stoppedDay))
		if err != nil {
			return err
		}
		if len(today) != 0 || len(tomorrow) != 1 || tomorrow[0].BreakMinutes != 5 {
			t.Fatalf("已停用的计划应在次日重新启用，保留最近一次编辑: 今日 %+v，次日 %+v", today, tomorrow)
		}
		managed, err := plans.Management(db, stoppedDay)
		if err != nil {
			return err
		}
		if len(managed) != 1 || managed[0].ManagedActive || !managed[0].PendingStatus {
			t.Fatalf("待启用的计划应留在停用列表并可撤销: %+v", managed)
		}
		if err := plans.ChangeStatus(db, original.ID, plans.Stopped, stoppedDay); err != nil {
			return err
		}
		managed, err = plans.Management(db, stoppedDay)
		if err != nil {
			return err
		}
		if len(managed) != 1 || managed[0].ManagedActive || managed[0].Active || managed[0].PendingStatus {
			t.Fatalf("撤销启用后应保持停用并清除待启用状态: %+v", managed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScheduleHistoryDistinguishesLegacyUnknownAndComplete(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now().In(studycalendar.Location)
	legacy := studycalendar.Day(now.AddDate(0, 0, -2))
	unknown := studycalendar.Day(now.AddDate(0, 0, -1))
	complete := studycalendar.Day(now)
	if _, err := testExec(app, `INSERT INTO plan_versions(kind,effective_day,legacy_targets_json) VALUES('legacy_target',?,'[{"subjectId":1,"subjectName":"数学","plannedSeconds":7200}]')`, legacy); err != nil {
		t.Fatal(err)
	}
	err := app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		items, err := plans.DailyPlans(a.Queries(context.Background()), []string{legacy, unknown, complete}, now)
		if err != nil {
			return err
		}
		if items[legacy].Kind != "legacy" || len(items[legacy].TargetsJSON) == 0 ||
			items[unknown].Kind != "unknown" || items[complete].Kind != "complete" || len(items[complete].Plans) != 0 {
			t.Fatalf("历史类型混淆: %+v", items)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScheduleHistoryMigratesLegacyIDsAndEmptyTarget(t *testing.T) {
	paths := startupPathsFixture(t)
	now := time.Now().In(studycalendar.Location)
	day := studycalendar.Day(now)
	legacyDay := studycalendar.Day(now.AddDate(0, 0, -1))
	dbPath := filepath.Join(paths.defaultDir, "study.db")
	seedStartupSQLite(t, dbPath, true, `
		INSERT INTO subjects(id,name,created_at) VALUES(91,'迁移项目','2026-01-01');
		INSERT INTO plans(id,subject_id,start_time,duration_minutes,weekdays,break_minutes,note)
		VALUES(209,91,'23:00',40,'0,1,2,3,4,5,6',5,'保留备注');
		INSERT INTO sessions(id,subject_id,plan_id,start_at,source) VALUES(27,91,209,'2026-01-01T23:00:00+08:00','timer');
		INSERT INTO daily_plan_targets(day,targets_json) VALUES('`+legacyDay+`','[]');
	`)
	app, startup := prepareStartupUpgrade(paths)
	defer app.Close()
	startup.activate = func() error { return nil }
	state := (&StartupService{control: startup}).Confirm()
	if state.Phase != "ready" {
		t.Fatalf("升级失败: %+v", state)
	}
	err := app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		db := a.Queries(context.Background())
		items, err := plans.DailyPlans(db, []string{legacyDay, day}, now)
		if err != nil {
			return err
		}
		legacy := items[legacyDay]
		today := items[day]
		if legacy.Kind != "legacy" || string(legacy.TargetsJSON) != "[]" {
			t.Fatalf("空目标快照丢失: %+v", legacy)
		}
		if !today.AdoptionDay || len(today.Plans) != 1 {
			t.Fatalf("升级日排程异常: %+v", today)
		}
		plan := today.Plans[0]
		if plan.ID != 209 || plan.SubjectID != 91 || plan.BreakMinutes != 5 || plan.Note != "保留备注" {
			t.Fatalf("计划内容或稳定 ID 变化: %+v", plan)
		}
		var sessionPlanID, sessionVersionID int64
		if err := db.QueryRow(`SELECT plan_id,plan_version_id FROM sessions WHERE id=27`).Scan(&sessionPlanID, &sessionVersionID); err != nil {
			return err
		}
		if sessionPlanID != plan.ID || sessionVersionID != plan.VersionID {
			t.Fatalf("计时关联异常: %d / %d, %+v", sessionPlanID, sessionVersionID, plan)
		}
		var oldTables int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table'
			AND name IN ('subjects','plans','daily_plan_targets','schedule_history')`).Scan(&oldTables); err != nil {
			return err
		}
		if oldTables != 0 {
			t.Fatalf("旧表残留: %d", oldTables)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScheduleHistoryRejectsUnknownSubjectInStoppedPlan(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now()
	err := app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		db := a.Queries(context.Background())
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "校验项目"}, true, now)
		if err != nil {
			return err
		}
		plan, err := plans.SaveVersion(db, plans.Plan{
			SubjectID: subject.ID, StartTime: "09:00", Duration: 60, Weekdays: "0,1,2,3,4,5,6",
		}, true, now)
		if err != nil {
			return err
		}
		if err = plans.ChangeStatus(db, plan.ID, plans.Stopped, now); err != nil {
			return err
		}
		plan.SubjectID = subject.ID + 1000
		if _, err := plans.SaveVersion(db, plan, false, now); err == nil {
			t.Error("停用计划错误地接受了不存在的项目")
		}
		var orphaned bool
		err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM plan_versions WHERE plan_id=? AND subject_id=?)`,
			plan.ID, plan.SubjectID).Scan(&orphaned)
		if orphaned {
			t.Error("保存失败后留下孤立的计划版本")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestScheduleHistoryStatusKeepsInheritedPlanColor(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now()
	planDay := now.In(studycalendar.Location).AddDate(0, 0, 1)
	err := app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
		db := a.Queries(context.Background())
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "继承颜色", Color: "#007AFF"}, true, now)
		if err != nil {
			return err
		}
		plan, err := plans.SaveVersion(db, plans.Plan{
			SubjectID: subject.ID, StartTime: "09:00", Duration: 60, Weekdays: "0,1,2,3,4,5,6",
		}, true, now)
		if err != nil {
			return err
		}
		subject.Color = "#34C759"
		if _, err = subjects.SaveVersion(db, subject, false, planDay); err != nil {
			return err
		}
		for _, status := range []string{plans.Stopped, plans.Active} {
			if err = plans.ChangeStatus(db, plan.ID, status, planDay); err != nil {
				return err
			}
			var storedColor string
			err = db.QueryRow(`SELECT color FROM plan_versions WHERE plan_id=? ORDER BY id DESC LIMIT 1`, plan.ID).Scan(&storedColor)
			if err != nil {
				return err
			}
			if storedColor != "" {
				t.Errorf("%s后把继承色误存成固定值 %q", status, storedColor)
			}
		}
		items, err := plans.AsOf(db, studycalendar.Tomorrow(planDay))
		if err != nil {
			return err
		}
		if len(items) != 1 || items[0].Color != subject.Color {
			t.Fatalf("显示色应继续跟随项目: %+v", items)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPlanConfirmationRequiresCurrentEffectiveDay(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	ctx := context.Background()
	opened := time.Date(2026, time.November, 23, 23, 59, 0, 0, studycalendar.Location)
	submitted := opened.Add(2 * time.Minute)
	err := app.database.WithAccess(ctx, func(access *persistence.Access) error {
		db := access.Queries(ctx)
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "跨日确认"}, true, opened)
		if err != nil {
			return err
		}
		plan := plans.Plan{SubjectID: subject.ID, StartTime: "20:00", Duration: 60, Weekdays: "1,3"}
		submit := func(method string, path string, day string, body any) *httptest.ResponseRecorder {
			t.Helper()
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(method, path+day, bytes.NewReader(data))
			response := httptest.NewRecorder()
			app.versionedPlans(db, response, req, submitted)
			return response
		}
		assertPending := func(response *httptest.ResponseRecorder) {
			t.Helper()
			var result struct {
				NeedsConfirmation bool
				EffectiveDay      string
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
				!result.NeedsConfirmation || result.EffectiveDay != "2026-11-25" {
				t.Fatalf("expected a new confirmation, got %s (%v)", response.Body.String(), err)
			}
		}
		path := "/plans?expectedEffectiveDay="
		assertPending(submit(http.MethodPost, path, "2026-11-24", plan))
		asOf, err := plans.AsOf(db, "2026-11-25")
		if err != nil || len(asOf) != 0 {
			t.Fatalf("stale confirmation wrote a plan: %+v (%v)", asOf, err)
		}

		saved := submit(http.MethodPost, path, "2026-11-25", plan)
		if err := json.Unmarshal(saved.Body.Bytes(), &plan); err != nil || plan.VersionID == 0 || plan.EffectiveDay != "2026-11-25" {
			t.Fatalf("reconfirmed plan should be saved: %s (%v)", saved.Body.String(), err)
		}
		plan.Weekdays = "3,1"
		updated := submit(http.MethodPut, path, "2026-11-25", plan)
		var unchanged plans.Plan
		if err := json.Unmarshal(updated.Body.Bytes(), &unchanged); err != nil ||
			!unchanged.Unchanged || unchanged.VersionID != plan.VersionID {
			t.Fatalf("same plan should retain its version: %s (%v)", updated.Body.String(), err)
		}
		deletePath := "/plans?id=" + strconv.FormatInt(plan.ID, 10) + "&expectedEffectiveDay="
		assertPending(submit(http.MethodDelete, deletePath, "2026-11-24", nil))
		versions, err := plans.HistoryVersions(db, plan.ID)
		if err != nil || len(versions) != 1 {
			t.Fatalf("expected only the new plan version: %+v (%v)", versions, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
