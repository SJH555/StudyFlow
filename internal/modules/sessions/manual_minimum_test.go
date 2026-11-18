package sessions

import (
	"exam_notify/internal/testutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func TestManualMinimumDuration(t *testing.T) {
	for _, duration := range []time.Duration{5 * time.Minute, 599 * time.Second, 600*time.Second - time.Millisecond, 10 * time.Minute, 601 * time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			db := sessionTestDB(t)
			start := time.Date(2026, 11, 17, 13, 0, 0, 0, time.Local)
			id, seconds, err := AddManual(db, ManualInput{SubjectID: 1, StartAt: start.Format(time.RFC3339Nano), EndAt: start.Add(duration).Format(time.RFC3339Nano)})
			var count int
			if queryErr := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&count); queryErr != nil {
				t.Fatal(queryErr)
			}
			if duration < 10*time.Minute {
				if err == nil || !strings.Contains(err.Error(), "不能少于 10 分钟") || id != 0 || count != 0 {
					t.Fatalf("short record: id=%d count=%d err=%v", id, count, err)
				}
			} else {
				if err != nil || count != 1 || seconds != int64(duration.Seconds()) {
					t.Fatalf("valid record: count=%d seconds=%d err=%v", count, seconds, err)
				}
				var source string
				if err := db.QueryRow(`SELECT source FROM sessions WHERE id=?`, id).Scan(&source); err != nil || source != "manual" {
					t.Fatalf("source=%q err=%v", source, err)
				}
			}
		})
	}
}

func TestLegacyShortManualRecordNoLongerBlocksAfterStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-17');
		INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source,note) VALUES
		(1,'2026-11-17T09:00:00+08:00','2026-11-17T09:05:00+08:00',300,'manual','保留原始内容'),
		(1,'2026-11-17T10:00:00+08:00','2026-11-17T10:10:00+08:00',600,'manual','有效补录')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = persistence.OpenForStartup(path)
	if err != nil {
		t.Fatal(err)
	}
	q := testutil.Borrow(t, db, path)
	if _, _, err := AddManual(q, ManualInput{SubjectID: 1, StartAt: "2026-11-17T09:00:00+08:00", EndAt: "2026-11-17T09:10:00+08:00"}); err != nil {
		t.Fatalf("invisible legacy record still blocks: %v", err)
	}
	if _, _, err := AddManual(q, ManualInput{SubjectID: 1, StartAt: "2026-11-17T10:00:00+08:00", EndAt: "2026-11-17T10:10:00+08:00"}); err == nil || !strings.Contains(err.Error(), "时间冲突") {
		t.Fatalf("valid record overlap no longer blocked: %v", err)
	}
	var deleted int
	var note string
	if err := db.QueryRow(`SELECT deleted,note FROM sessions WHERE duration_seconds=300`).Scan(&deleted, &note); err != nil || deleted != 1 || note != "保留原始内容" {
		t.Fatalf("legacy row not preserved: deleted=%d note=%q err=%v", deleted, note, err)
	}
}
