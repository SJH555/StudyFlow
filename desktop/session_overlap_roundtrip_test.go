package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/sessions"
	"exam_notify/internal/modules/subjects"
	"exam_notify/internal/shared/studycalendar"
)

func overlapFixture(t *testing.T) (*App, time.Time, sessions.TimerInput) {
	t.Helper()
	_, app, _ := prepareUpgradeFixture(t, false)
	now := time.Now().In(studycalendar.Location)
	created := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())
	var timer sessions.TimerInput
	ctx := context.Background()
	err := app.database.WithAccess(ctx, func(a *persistence.Access) error {
		db := a.Queries(ctx)
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "数学"}, true, created)
		if err != nil {
			return err
		}
		p, err := plans.SaveVersion(db, plans.Plan{
			SubjectID: subject.ID, StartTime: "10:00", Duration: 60, Weekdays: "0,1,2,3,4,5,6",
		}, true, created)
		timer = sessions.TimerInput{PlanID: p.ID, SubjectID: subject.ID}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, created.AddDate(0, 0, 1), timer
}

func TestManualPlanConflictHTTPAndTimerRecheck(t *testing.T) {
	app, start, timer := overlapFixture(t)
	manual := sessions.ManualInput{SubjectID: timer.SubjectID,
		StartAt: start.Add(15 * time.Minute).Format(time.RFC3339),
		EndAt:   start.Add(45 * time.Minute).Format(time.RFC3339),
	}
	post := func(in sessions.ManualInput) (int, string) {
		body, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(string(body)))
		testRoute(app, app.sessions, w, r)
		return w.Code, w.Body.String()
	}

	status, body := post(manual)
	var result struct {
		PlanConflict *sessions.ManualPlanConflict
	}
	if status != 200 || json.Unmarshal([]byte(body), &result) != nil || result.PlanConflict == nil ||
		!strings.Contains(result.PlanConflict.Message, "数学") || !strings.Contains(result.PlanConflict.Message, "10:15–10:45") {
		t.Fatalf("expected plan warning: %d %s", status, body)
	}
	if got := upgradeScalar(t, app.database, "SELECT count(*) FROM sessions"); got != "0" {
		t.Fatalf("warning wrote %s records", got)
	}
	manual.PlanConflictToken = result.PlanConflict.Token
	if status, body := post(manual); status != 200 || !strings.Contains(body, "duration") {
		t.Fatalf("confirmed save failed: %d %s", status, body)
	}

	ctx := context.Background()
	if err := app.database.WithAccess(ctx, func(a *persistence.Access) error {
		db := a.Queries(ctx)
		for _, now := range []time.Time{start.Add(-time.Minute), start} {
			if _, err := sessions.Start(db, timer, now); err == nil || !strings.Contains(err.Error(), "时间冲突") {
				t.Fatalf("conflicting timer accepted: %v", err)
			}
		}
		id, err := sessions.Start(db, timer, start.Add(45*time.Minute))
		if err != nil {
			return err
		}
		saved, err := sessions.ChangeTimer(db, fmt.Sprint(id), "stop", start.Add(time.Hour))
		if err != nil || !saved {
			t.Fatalf("adjacent timer could not finish: saved=%t err=%v", saved, err)
		}
		actual, err := sessions.StudySecondsBySubject(db, studycalendar.Day(start))
		if err == nil && actual[timer.SubjectID] != 45*60 {
			t.Fatalf("actual seconds=%d, want 2700", actual[timer.SubjectID])
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmedManualConflictRemainsSafeWhenTimerStarts(t *testing.T) {
	app, start, timer := overlapFixture(t)
	manual := sessions.ManualInput{SubjectID: timer.SubjectID,
		StartAt: start.Add(15 * time.Minute).Format(time.RFC3339),
		EndAt:   start.Add(45 * time.Minute).Format(time.RFC3339),
	}
	ctx := context.Background()
	if err := app.database.WithAccess(ctx, func(a *persistence.Access) error {
		_, _, err := sessions.AddManual(a.Queries(ctx), manual)
		var warning *sessions.ManualPlanConflict
		if !errors.As(err, &warning) {
			t.Fatalf("expected warning, got %v", err)
		}
		manual.PlanConflictToken = warning.Token
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	ready := make(chan struct{})
	results := make(chan error, 2)
	for _, write := range []func(*persistence.Queries) error{
		func(q *persistence.Queries) error { _, _, err := sessions.AddManual(q, manual); return err },
		func(q *persistence.Queries) error { _, err := sessions.Start(q, timer, start); return err },
	} {
		go func(write func(*persistence.Queries) error) {
			<-ready
			results <- app.database.WithAccess(ctx, func(a *persistence.Access) error { return write(a.Queries(ctx)) })
		}(write)
	}
	close(ready)
	var successes int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if !strings.Contains(err.Error(), "时间冲突") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if got := upgradeScalar(t, app.database, "SELECT count(*) FROM sessions"); got != "1" || successes != 1 {
		t.Fatalf("successes=%d, sessions=%s", successes, got)
	}
}
