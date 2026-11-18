package persistence

import (
	"path/filepath"
	"testing"
)

func TestStartupShortManualCleanupIsScopedAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-11-17');
		INSERT INTO sessions(id,subject_id,start_at,end_at,duration_seconds,source) VALUES
		(1,1,'2026-11-17T09:00:00+08:00','2026-11-17T09:05:00+08:00',300,'manual'),
		(2,1,'2026-11-17T10:00:00+08:00','2026-11-17T10:10:00+08:00',600,'manual'),
		(3,1,'2026-11-17T11:00:00+08:00',NULL,0,'timer'),
		(4,1,'2026-11-17T12:00:00+08:00',NULL,0,'manual'),
		(5,1,'2026-11-17T13:00:00+08:00','2026-11-17T13:05:00+08:00',300,'timer')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		db, err = OpenForStartup(path)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 5 {
			t.Fatalf("original rows removed: count=%d err=%v", count, err)
		}
		for id := 1; id <= 5; id++ {
			var deleted int
			if err := db.QueryRow(`SELECT deleted FROM sessions WHERE id=?`, id).Scan(&deleted); err != nil {
				t.Fatal(err)
			}
			want := 0
			if id == 1 {
				want = 1
			}
			if deleted != want {
				t.Fatalf("id=%d deleted=%d want=%d", id, deleted, want)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
