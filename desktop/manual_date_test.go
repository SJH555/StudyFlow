package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/subjects"
	"exam_notify/internal/shared/studycalendar"
)

func TestManualRecordSubjectOptionsFollowSelectedDate(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	ctx := context.Background()
	today := time.Now().In(studycalendar.Location)
	tomorrow := studycalendar.Tomorrow(today)
	err := app.database.WithAccess(ctx, func(access *persistence.Access) error {
		db := access.Queries(ctx)
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "今日名称"}, true, today)
		if err != nil {
			return err
		}
		subject.Name = "明日名称"
		if _, err := subjects.SaveVersion(db, subject, false, today); err != nil {
			return err
		}
		for day, expected := range map[string]string{studycalendar.Day(today): "今日名称", tomorrow: "明日名称"} {
			req := httptest.NewRequest(http.MethodGet, "/subjects?manualDate="+day, nil)
			response := httptest.NewRecorder()
			app.subjects(db, response, req)
			var options []Subject
			if err := json.Unmarshal(response.Body.Bytes(), &options); err != nil ||
				len(options) != 1 || options[0].Name != expected {
				t.Fatalf("%s subject options: %s (%v)", day, response.Body.String(), err)
			}
		}
		req := httptest.NewRequest(http.MethodGet, "/subjects?manualDate=2026-02-30", nil)
		response := httptest.NewRecorder()
		app.subjects(db, response, req)
		if response.Code == http.StatusOK {
			t.Fatalf("invalid date returned success: %s", response.Body.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestManualRecordSaveFollowsSubjectAvailabilityOnSelectedDate(t *testing.T) {
	_, app, _ := prepareUpgradeFixture(t, false)
	ctx := context.Background()
	now := time.Now().In(studycalendar.Location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 1, 30, 0, 0, now.Location())
	tomorrow := today.AddDate(0, 0, 1)
	err := app.database.WithAccess(ctx, func(access *persistence.Access) error {
		db := access.Queries(ctx)
		subject, err := subjects.SaveVersion(db, subjects.Subject{Name: "明天停用"}, true, today)
		if err != nil {
			return err
		}
		subject.Archived = true
		if _, err := subjects.SaveVersion(db, subject, false, today); err != nil {
			return err
		}

		for _, day := range []time.Time{today, tomorrow} {
			key := studycalendar.Day(day)
			response := httptest.NewRecorder()
			app.subjects(db, response, httptest.NewRequest(http.MethodGet, "/subjects?manualDate="+key, nil))
			var options []Subject
			if err := json.Unmarshal(response.Body.Bytes(), &options); response.Code != http.StatusOK || err != nil {
				t.Fatalf("%s options: %d %s (%v)", key, response.Code, response.Body.String(), err)
			}
			available := day.Equal(today)
			if (len(options) == 1 && options[0].ID == subject.ID) != available {
				t.Fatalf("%s unexpected options: %+v", key, options)
			}

			// 使用 UTC 序列化，此时北京时间次日的开始时间在请求里仍是前一天。
			body := fmt.Sprintf(`{"subjectId":%d,"startAt":%q,"endAt":%q}`,
				subject.ID, day.UTC().Format(time.RFC3339), day.Add(time.Hour).UTC().Format(time.RFC3339))
			response = httptest.NewRecorder()
			app.sessions(db, response, httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(body)))
			if (response.Code == http.StatusOK) != available {
				t.Fatalf("%s save: %d %s", key, response.Code, response.Body.String())
			}
			if !available && !strings.Contains(response.Body.String(), "不可用于补录") {
				t.Fatalf("missing availability error: %s", response.Body.String())
			}
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("expected exactly one saved session, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
