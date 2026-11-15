package persistence

import (
	"path/filepath"
	"testing"
)

func TestStartupAddsGoalSchemaAndPreservesExistingData(t *testing.T) {
	for _, profile := range []string{"without-goals", "without-acceptance"} {
		t.Run(profile, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			statements := []string{
				`INSERT INTO subjects(id,name,color,created_at) VALUES(71,'升级保留项目','#34C759','2026-11-01')`,
				`INSERT INTO plans(id,subject_id,start_time,duration_minutes,note) VALUES(72,71,'09:00',30,'原计划备注')`,
				`INSERT INTO sessions(id,subject_id,plan_id,start_at,end_at,duration_seconds,note) VALUES(73,71,72,'2026-11-01T09:00:00+08:00','2026-11-01T09:20:00+08:00',1200,'原记录备注')`,
				`INSERT INTO activity_logs(id,happened_at,content) VALUES(74,'2026-11-01T10:00:00+08:00','原活动')`,
				`INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at) VALUES('2026-11-01','project',80,1800,1200,'2026-11-02T08:00:00+08:00')`,
				`INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason) VALUES('2026-11-01',71,'升级保留项目',1800,1200,'原总结')`,
				`INSERT INTO daily_plan_targets(day,targets_json) VALUES('2026-11-01','[{"subjectId":71,"subjectName":"升级保留项目","plannedSeconds":1800}]')`,
				`INSERT INTO settings(key,value) VALUES('ringtoneVolume','35')`,
			}
			if profile == "without-goals" {
				statements = append(statements, `DROP TABLE stage_goals`)
			} else {
				statements = append(statements,
					`ALTER TABLE stage_goals DROP COLUMN accepted_at`,
					`INSERT INTO stage_goals(id,title,deadline,note,completed_at,created_at,updated_at) VALUES(75,'原已完成目标','2026-11-10','原目标备注','2026-11-09T12:00:00+08:00','2026-11-01T08:00:00+08:00','2026-11-09T12:00:00+08:00')`)
			}
			for _, statement := range statements {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = OpenForStartup(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			checks := map[string]string{
				`SELECT name||':'||color FROM subjects WHERE id=71`:                                                 "升级保留项目:#34C759",
				`SELECT note FROM plans WHERE id=72 AND subject_id=71`:                                              "原计划备注",
				`SELECT note FROM sessions WHERE id=73 AND subject_id=71 AND plan_id=72 AND duration_seconds=1200`:  "原记录备注",
				`SELECT content FROM activity_logs WHERE id=74`:                                                     "原活动",
				`SELECT mode FROM daily_reviews WHERE day='2026-11-01' AND actual_seconds=1200`:                     "project",
				`SELECT reason FROM daily_review_items WHERE day='2026-11-01' AND subject_id=71`:                    "原总结",
				`SELECT json_extract(targets_json,'$[0].subjectId') FROM daily_plan_targets WHERE day='2026-11-01'`: "71",
				`SELECT value FROM settings WHERE key='ringtoneVolume'`:                                             "35",
				`SELECT count(*) FROM pragma_table_info('stage_goals') WHERE name='accepted_at'`:                    "1",
			}
			if profile == "without-acceptance" {
				checks[`SELECT title||':'||note FROM stage_goals WHERE id=75 AND accepted_at=completed_at`] = "原已完成目标:原目标备注"
			}
			for query, want := range checks {
				var got string
				if err := db.QueryRow(query).Scan(&got); err != nil || got != want {
					t.Errorf("%s: got %q, %v; want %q", query, got, err, want)
				}
			}
			rows, err := db.Query(`PRAGMA foreign_key_check`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if rows.Next() {
				t.Fatal("升级后存在失效外键")
			}
		})
	}
}
