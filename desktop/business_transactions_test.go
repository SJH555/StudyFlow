package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func businessFixture(t *testing.T, api *APIService, query string, args ...any) {
	t.Helper()
	if _, err := testExec(api.app, query, args...); err != nil {
		t.Fatal(err)
	}
}

func businessCount(t *testing.T, api *APIService, query string, args ...any) int {
	t.Helper()
	var count int
	if err := testQueryRow(api.app, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func businessAPIError(t *testing.T, api *APIService, path, method string, body any) string {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response := api.Call(path, method, string(payload))
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil || result.Error == "" {
		t.Fatalf("expected API error: %s", response)
	}
	return result.Error
}

func TestBusinessTransactionSubjectLinkFailureRollsBack(t *testing.T) {
	for _, action := range []string{"archive", "delete"} {
		t.Run(action, func(t *testing.T) {
			api := newAPITestService(t)
			businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'original','2026-11-18');
		INSERT INTO plans(subject_id,start_time,duration_minutes,deleted) VALUES(1,'09:00',60,1);
		CREATE TRIGGER reject_plan_active BEFORE UPDATE OF active ON plans BEGIN SELECT RAISE(ABORT,'injected linked plan failure'); END`)
			if action == "archive" {
				businessAPIError(t, api, "subjects", http.MethodPut, map[string]any{"id": 1, "name": "changed", "archived": true})
			} else {
				businessAPIError(t, api, "subjects?id=1", http.MethodDelete, nil)
			}
			if got := businessCount(t, api, `SELECT count(*) FROM subjects WHERE id=1 AND name='original' AND archived=0 AND deleted=0`); got != 1 {
				t.Fatal("subject partially updated")
			}
			businessFixture(t, api, `DROP TRIGGER reject_plan_active`)
			if got := api.Call("subjects", http.MethodGet, ""); strings.Contains(got, `"error"`) {
				t.Fatal(got)
			}
		})
	}
}

func TestBusinessTransactionStartRejectsMalformedOldSession(t *testing.T) {
	for _, column := range []string{"duration_seconds", "planned_end_at", "start_at"} {
		t.Run(column, func(t *testing.T) {
			api := newAPITestService(t)
			businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18'); INSERT INTO sessions(id,subject_id,start_at) VALUES(1,1,?)`, time.Now().Add(-20*time.Minute).Format(time.RFC3339))
			businessFixture(t, api, "UPDATE sessions SET "+column+"='malformed' WHERE id=1")
			businessAPIError(t, api, "sessions/0", http.MethodPost, map[string]any{"action": "start", "subjectId": 1})
			if n := businessCount(t, api, `SELECT count(*) FROM sessions WHERE end_at IS NULL AND deleted=0`); n != 1 {
				t.Fatalf("active sessions=%d", n)
			}
		})
	}
}

func TestBusinessTransactionListsRejectIncompleteResults(t *testing.T) {
	for _, route := range []string{"subjects", "plans", "sessions", "settings", "settings_iteration"} {
		t.Run(route, func(t *testing.T) {
			api := newAPITestService(t)
			businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18')`)
			switch route {
			case "subjects":
				businessFixture(t, api, `UPDATE subjects SET sort_order='malformed' WHERE id=1`)
			case "plans":
				businessFixture(t, api, `INSERT INTO plans(subject_id,start_time,duration_minutes) VALUES(1,'09:00','malformed')`)
			case "sessions":
				businessFixture(t, api, `INSERT INTO sessions(subject_id,start_at,duration_seconds) VALUES(1,?,'malformed')`, time.Now().Format(time.RFC3339))
			case "settings":
				businessFixture(t, api, `DROP TABLE settings; CREATE TABLE settings(key TEXT,value TEXT); INSERT INTO settings VALUES('bad',NULL)`)
			case "settings_iteration":
				// A valid first row followed by SQLite's integer-overflow step error.
				businessFixture(t, api, `ALTER TABLE settings RENAME TO settings_original; CREATE VIEW settings AS SELECT 'first' AS key,'ok' AS value UNION ALL SELECT 'second',abs(-9223372036854775808)`)
			}
			path := route
			if route == "settings_iteration" {
				path = "settings"
			}
			businessAPIError(t, api, path, http.MethodGet, nil)
			if got := api.Call("snapshot", http.MethodGet, ""); strings.Contains(got, `"error"`) {
				t.Fatalf("connection unavailable after query error: %s", got)
			}
		})
	}
}

func TestBusinessTransactionGoalReadbackFailureRollsBack(t *testing.T) {
	api := newAPITestService(t)
	stamp := time.Now().Format(time.RFC3339)
	businessFixture(t, api, `INSERT INTO stage_goals(id,title,deadline,created_at,updated_at) VALUES(1,'original','2026-12-01',?,?);
	CREATE TRIGGER corrupt_goal_readback AFTER UPDATE OF title ON stage_goals BEGIN UPDATE stage_goals SET created_at='malformed' WHERE id=NEW.id; END`, stamp, stamp)
	businessAPIError(t, api, "goals/1", http.MethodPut, map[string]any{"title": "changed", "deadline": "2026-12-02"})
	if n := businessCount(t, api, `SELECT count(*) FROM stage_goals WHERE id=1 AND title='original' AND created_at=?`, stamp); n != 1 {
		t.Fatal("goal update committed before readback validation")
	}
}

func TestBusinessTransactionSettingsBatchFailureSkipsCallbacks(t *testing.T) {
	api := newAPITestService(t)
	callbacks := 0
	api.app.onActivityReminderChanged = func(bool) { callbacks++ }
	businessFixture(t, api, `INSERT OR REPLACE INTO settings(key,value) VALUES('activityReminderEnabled','0');
	CREATE TRIGGER reject_second_setting BEFORE INSERT ON settings WHEN NEW.key IN ('qa_batch_a','qa_batch_b') AND EXISTS(SELECT 1 FROM settings WHERE key IN ('qa_batch_a','qa_batch_b')) BEGIN SELECT RAISE(ABORT,'injected second setting failure'); END`)
	businessAPIError(t, api, "settings", http.MethodPut, map[string]string{"qa_batch_a": "a", "qa_batch_b": "b", "activityReminderEnabled": "1"})
	if n := businessCount(t, api, `SELECT count(*) FROM settings WHERE key IN ('qa_batch_a','qa_batch_b') OR (key='activityReminderEnabled' AND value='1')`); n != 0 || callbacks != 0 {
		t.Fatalf("partial settings=%d callbacks=%d", n, callbacks)
	}
}

func TestBusinessTransactionSettingsCallbackCanReborrow(t *testing.T) {
	api := newAPITestService(t)
	callbackResult := make(chan error, 1)
	api.app.onActivityReminderChanged = func(enabled bool) {
		var value string
		err := testQueryRow(api.app, `SELECT value FROM settings WHERE key='activityReminderEnabled'`).Scan(&value)
		if err == nil && (!enabled || value != "1") {
			err = fmt.Errorf("callback saw uncommitted value %q", value)
		}
		callbackResult <- err
	}
	result := make(chan string, 1)
	go func() { result <- api.Call("settings", http.MethodPut, `{"activityReminderEnabled":"1"}`) }()
	if got := readAPIResult(t, result); strings.Contains(got, `"error"`) {
		t.Fatal(got)
	}
	if err := readAPIResult(t, callbackResult); err != nil {
		t.Fatal(err)
	}
}

func reviewTransactionFixture(t *testing.T, api *APIService, days int) (string, string) {
	t.Helper()
	today := time.Now()
	first := today.AddDate(0, 0, -days).Format(time.DateOnly)
	last := today.AddDate(0, 0, -1).Format(time.DateOnly)
	businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18'); INSERT INTO plans(subject_id,start_time,duration_minutes) VALUES(1,'09:00',60);
	UPDATE settings SET value='1' WHERE key='dailyReviewEnabled'; UPDATE settings SET value='project' WHERE key='dailyReviewMode';
	INSERT INTO settings(key,value) VALUES('dailyReviewCheckHour','0'),('dailyReviewCheckMinute','0') ON CONFLICT(key) DO UPDATE SET value=excluded.value; UPDATE settings SET value=? WHERE key='dailyReviewStartDate'`, first)
	return first, last
}

func TestBusinessTransactionReviewFailureRollsBackHeadersItemsAndSnapshots(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(fmt.Sprintf("bulk=%t", bulk), func(t *testing.T) {
			api := newAPITestService(t)
			days := 1
			if bulk {
				days = 5
			}
			first, last := reviewTransactionFixture(t, api, days)
			if bulk {
				businessFixture(t, api, `CREATE TRIGGER reject_later_reason BEFORE INSERT ON daily_review_items WHEN NEW.day<>'`+first+`' BEGIN SELECT RAISE(ABORT,'injected later reason failure'); END`)
			} else {
				businessFixture(t, api, `CREATE TRIGGER reject_reason BEFORE INSERT ON daily_review_items BEGIN SELECT RAISE(ABORT,'injected reason failure'); END`)
			}
			if bulk {
				businessAPIError(t, api, "daily-review/bulk", http.MethodPost, map[string]any{"startDay": first, "endDay": last, "reason": "test"})
			} else {
				businessAPIError(t, api, "daily-review/submit", http.MethodPost, map[string]any{"day": first, "reasons": []map[string]any{{"subjectId": 1, "reason": "test"}}})
			}
			for _, table := range []string{"daily_reviews", "daily_review_items", "daily_plan_targets"} {
				if n := businessCount(t, api, "SELECT count(*) FROM "+table); n != 0 {
					t.Fatalf("partial %s rows=%d", table, n)
				}
			}
		})
	}
}

func TestBusinessTransactionFinishExpiredRollsBackEarlierSession(t *testing.T) {
	api := newAPITestService(t)
	now := time.Now()
	start := now.Add(-time.Hour).Format(time.RFC3339)
	end := now.Add(-time.Minute).Format(time.RFC3339)
	businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18'); INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'09:00',60);
	INSERT INTO sessions(id,subject_id,plan_id,start_at,planned_end_at) VALUES(1,1,1,?,?),(2,1,1,?,?);
	CREATE TRIGGER reject_second_end BEFORE UPDATE ON sessions WHEN OLD.id=2 BEGIN SELECT RAISE(ABORT,'injected second timer failure'); END`, start, end, start, end)
	alert, err := api.app.scheduler.PlanReminder(now, false)
	if err == nil || alert != nil {
		t.Fatalf("result=%+v err=%v", alert, err)
	}
	if n := businessCount(t, api, `SELECT count(*) FROM sessions WHERE end_at IS NULL AND deleted=0`); n != 2 {
		t.Fatalf("partially finished sessions=%d", n)
	}
}

func TestBusinessTransactionConcurrentConflictingPlans(t *testing.T) {
	api := newAPITestService(t)
	businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18')`)
	results := make(chan string, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- api.Call("plans", http.MethodPost, `{"subjectId":1,"startTime":"09:00","duration":60}`)
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for result := range results {
		if !strings.Contains(result, `"error"`) {
			success++
		}
	}
	if n := businessCount(t, api, `SELECT count(*) FROM plans`); success != 1 || n != 1 {
		t.Fatalf("success=%d plans=%d", success, n)
	}
}

func TestBusinessTransactionActualCommitBusyPausesAPI(t *testing.T) {
	api := newAPITestService(t)
	path := api.app.database.Path()
	businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18'); INSERT INTO sessions(id,subject_id,start_at) VALUES(1,1,?)`, time.Now().Add(-20*time.Minute).Format(time.RFC3339))
	reader, err := persistence.OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	readTx, err := reader.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Rollback()
	var n int
	if err := readTx.QueryRow(`SELECT count(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	message := businessAPIError(t, api, "sessions/0", http.MethodPost, map[string]any{"action": "start", "subjectId": 1})
	if !strings.Contains(message, "commit transaction") || !strings.Contains(message, "locked") {
		t.Fatal(message)
	}
	if message := businessAPIError(t, api, "subjects", http.MethodGet, nil); !strings.Contains(message, "unavailable") {
		t.Fatal(message)
	}
	if err := readTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow(`SELECT count(*) FROM sessions WHERE id=1 AND end_at IS NULL`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("old timer not restored: n=%d err=%v", n, err)
	}
	if err := reader.QueryRow(`SELECT count(*) FROM sessions`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("new timer escaped rollback: n=%d err=%v", n, err)
	}
	if err := api.app.database.WithRead(func(*sql.DB, string) error { t.Error("paused borrower entered"); return nil }); err == nil {
		t.Fatal("paused read admitted")
	}
}

func TestBusinessTransactionMalformedPlanRollsBackPreviousTimer(t *testing.T) {
	for _, column := range []string{"start_time", "duration_minutes", "weekdays"} {
		t.Run(column, func(t *testing.T) {
			api := newAPITestService(t)
			businessFixture(t, api, `INSERT INTO subjects(id,name,created_at) VALUES(1,'study','2026-11-18'); INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'09:00',60); INSERT INTO sessions(id,subject_id,start_at) VALUES(1,1,?)`, time.Now().Add(-20*time.Minute).Format(time.RFC3339))
			value := "malformed"
			if column == "duration_minutes" {
				value = "0"
			}
			businessFixture(t, api, "UPDATE plans SET "+column+"=? WHERE id=1", value)
			businessAPIError(t, api, "sessions/0", http.MethodPost, map[string]any{"action": "start", "subjectId": 1, "planId": 1})
			if n := businessCount(t, api, `SELECT count(*) FROM sessions WHERE id=1 AND end_at IS NULL AND deleted=0`); n != 1 {
				t.Fatal("previous timer update was not rolled back")
			}
			if n := businessCount(t, api, `SELECT count(*) FROM sessions`); n != 1 {
				t.Fatal("new timer must not be saved")
			}
		})
	}
}

func TestBusinessTransactionSnapshotConstraintIsNotIgnored(t *testing.T) {
	api := newAPITestService(t)
	reviewTransactionFixture(t, api, 1)
	// Inject a duplicate between the existence check and insertion. A blanket
	// INSERT OR IGNORE would report success and commit the injected row.
	businessFixture(t, api, `CREATE TRIGGER duplicate_snapshot BEFORE INSERT ON daily_plan_targets BEGIN INSERT INTO daily_plan_targets(day,targets_json) VALUES(NEW.day,'[]'); END`)
	businessAPIError(t, api, "daily-review/pending", http.MethodGet, nil)
	if n := businessCount(t, api, `SELECT count(*) FROM daily_plan_targets`); n != 0 {
		t.Fatal("snapshot constraint error was ignored or left partial data")
	}
}
