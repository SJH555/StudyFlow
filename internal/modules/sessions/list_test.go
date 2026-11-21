package sessions

import (
	"reflect"
	"testing"
	"time"
)

func TestListForDatesPreservesRecordsAndLocalDayBreakBudgets(t *testing.T) {
	db := sessionTestDB(t)
	base := time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local)
	day := base.Format("2006-01-02")
	previousDay := base.AddDate(0, 0, -1).Format("2006-01-02")
	emptyDay := base.AddDate(0, 0, 3).Format("2006-01-02")
	plan, zeroPlan := int64(1), int64(0)
	makeRecord := func(id int64, planID *int64, at time.Time, used int64) Session {
		end := at.Add(20 * time.Minute).Format(time.RFC3339Nano)
		return Session{
			ID: id, PlanID: planID, SubjectID: 1, Subject: "数学", Color: "#007AFF",
			StartAt: at.Format(time.RFC3339Nano), EndAt: &end,
			BreakUsedSeconds: used, Duration: 1200, Source: "timer",
		}
	}
	records := []Session{
		makeRecord(1, &plan, base.Add(-30*time.Minute), 10),
		makeRecord(2, &plan, base.Add(5*time.Minute).UTC(), 20),
		makeRecord(3, &plan, base.Add(10*time.Minute).In(time.FixedZone("偏移时区", -7*3600)), 30),
		makeRecord(4, &plan, base.Add(20*time.Minute).UTC(), 5),
		makeRecord(5, nil, base.Add(30*time.Minute), 7),
		makeRecord(6, &zeroPlan, base.Add(40*time.Minute), 9),
		makeRecord(7, &zeroPlan, base.Add(50*time.Minute), 2),
		makeRecord(8, nil, base.Add(time.Hour), 11),
	}
	planned := base.Add(2 * time.Hour).Format(time.RFC3339Nano)
	paused := base.Add(21 * time.Minute).Format(time.RFC3339Nano)
	resumed := base.Add(20 * time.Minute).Format(time.RFC3339Nano)
	records[3].EndAt = nil
	records[3].PlannedEndAt = &planned
	records[3].PausedAt = &paused
	records[3].BreakStartedAt = &paused
	records[3].ResumedAt = &resumed
	records[3].Duration = 42
	records[3].Note = "保留进行中记录及全部可空时间字段"
	records[4].Source = "manual"
	records[7].Source = "manual"
	for _, record := range records {
		deleted := record.ID == 2
		_, err := db.Exec(`INSERT INTO sessions(
			id,subject_id,plan_id,start_at,end_at,planned_end_at,paused_at,resumed_at,
			break_started_at,break_used_seconds,duration_seconds,source,note,deleted
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			record.ID, record.SubjectID, record.PlanID, record.StartAt, record.EndAt,
			record.PlannedEndAt, record.PausedAt, record.ResumedAt, record.BreakStartedAt,
			record.BreakUsedSeconds, record.Duration, record.Source, record.Note, deleted,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	// 删除的历史时段仍计入该计划的当日休息额度；前一天和 NULL 计划不混入累计。
	records[2].BreakUsedSeconds = 50
	records[3].BreakUsedSeconds = 55
	records[6].BreakUsedSeconds = 11
	want := map[string][]Session{
		day:         {records[7], records[6], records[5], records[4], records[3], records[2]},
		previousDay: {records[0]},
		emptyDay:    {},
	}
	got, err := ListForDates(db, []string{emptyDay, previousDay, day, day})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("批量查询未保留记录含义：\ngot=%+v\nwant=%+v", got, want)
	}
	for date, expected := range want {
		items, err := List(db, date)
		if err != nil || !reflect.DeepEqual(items, expected) {
			t.Fatalf("单日查询 %s 不一致：%+v，%v", date, items, err)
		}
	}
	invalid, err := List(db, "invalid")
	if err != nil || invalid == nil || len(invalid) != 0 {
		t.Fatalf("原单日非法日期应保持空数组行为：%+v，%v", invalid, err)
	}
}

func TestListForDatesValidatesAndBoundsRequest(t *testing.T) {
	db := sessionTestDB(t)
	for _, days := range [][]string{
		{"2026-02-30"}, {""}, {"2026-11-5"}, make([]string, maxSessionQueryDays+1),
	} {
		if _, err := ListForDates(db, days); err == nil {
			t.Fatalf("接受了无效查询：%v", days)
		}
	}
	items, err := ListForDates(db, nil)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("空日期集合应返回空对象：%v，%v", items, err)
	}
}

func TestListForDatesUsesLocalDayAcrossClockChanges(t *testing.T) {
	for _, monthDay := range []struct {
		month time.Month
		day   int
	}{{time.March, 8}, {time.November, 1}} {
		base := time.Date(2026, monthDay.month, monthDay.day, 0, 0, 0, 0, time.Local)
		t.Run(base.Format("2006-01-02"), func(t *testing.T) {
			db := sessionTestDB(t)
			next := base.AddDate(0, 0, 1)
			instants := []time.Time{
				base.Add(-15 * time.Minute), base.Add(15 * time.Minute),
				base.Add(90 * time.Minute), base.Add(150 * time.Minute),
				next.Add(-15 * time.Minute), next.Add(15 * time.Minute),
			}
			for index, instant := range instants {
				id := index + 1
				if _, err := db.Exec(`INSERT INTO sessions(
					id,subject_id,plan_id,start_at,end_at,break_used_seconds,source
				) VALUES(?,1,1,?,?,?,'timer')`, id,
					instant.UTC().Format(time.RFC3339Nano),
					instant.Add(10*time.Minute).UTC().Format(time.RFC3339Nano), id,
				); err != nil {
					t.Fatal(err)
				}
			}
			day := base.Format("2006-01-02")
			previousDay := base.AddDate(0, 0, -1).Format("2006-01-02")
			nextDay := next.Format("2006-01-02")
			groups, err := ListForDates(db, []string{previousDay, day, nextDay})
			if err != nil {
				t.Fatal(err)
			}
			middle := groups[day]
			if len(middle) != 4 || len(groups[previousDay]) != 1 || len(groups[nextDay]) != 1 {
				t.Fatalf("本地日期边界错误：%v", groups)
			}
			wantIDs := []int64{5, 4, 3, 2}
			wantUsed := []int64{14, 9, 5, 2}
			for index, record := range middle {
				if record.ID != wantIDs[index] || record.BreakUsedSeconds != wantUsed[index] {
					t.Fatalf("本地日累计错误：%+v", middle)
				}
			}
			if groups[nextDay][0].BreakUsedSeconds != 6 {
				t.Fatal("下一自然日累计了前一天的休息")
			}
		})
	}
}
