package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func schedulerExec(t *testing.T, database *persistence.Database, query string, args ...any) {
	t.Helper()
	if err := database.WithWrite(func(db *sql.DB, _ string) error {
		_, err := db.Exec(query, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerBackupIntervalBoundaryAndCleanupWhileDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	database, err := persistence.NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, "INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1')")
	start := time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)
	dir := filepath.Join(filepath.Dir(path), "backups")
	name := func(at time.Time) string { return "study-" + at.Format("20060102-150405") + ".db" }
	check := func(at time.Time, want ...string) {
		t.Helper()
		if err := scheduler.BackupCheck(at); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != len(want) {
			t.Fatalf("backup count at %v = %d; want %d", at, len(entries), len(want))
		}
		for i, entry := range entries {
			if entry.Name() != want[i] {
				t.Fatalf("backup at %v = %q; want %q", at, entry.Name(), want[i])
			}
		}
	}

	boundary := start.Add(48 * time.Hour)
	check(start, name(start))
	check(boundary.Add(-time.Second), name(start))
	check(boundary, name(boundary))

	recent := name(boundary.Add(time.Second))
	if err := os.WriteFile(filepath.Join(dir, recent), []byte("recent backup fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	schedulerExec(t, database, "UPDATE settings SET value='0' WHERE key='autoBackupEnabled'")
	check(boundary.Add(48*time.Hour-time.Second), name(boundary), recent)
	check(boundary.Add(48*time.Hour), recent)
	check(boundary.Add(48*time.Hour + time.Second))
}

func TestSchedulerAutomaticBackupCreatesUsableSnapshotAndRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	database, err := persistence.NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scheduler := NewScheduler(database)

	schedulerExec(t, database, "INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1')")
	schedulerExec(t, database, "INSERT INTO subjects(name,color,created_at) VALUES('备份测试','#007AFF','2026-11-04')")

	now := time.Now().Truncate(time.Second)
	if err := scheduler.BackupCheck(now); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(filepath.Dir(path), "backups")
	first := filepath.Join(dir, "study-"+now.Format("20060102-150405")+".db")
	backupDB, err := sql.Open("sqlite", first)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := backupDB.QueryRow(`SELECT count(*) FROM subjects WHERE name='备份测试'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	_ = backupDB.Close()
	if count != 1 {
		t.Fatalf("backup subject count = %d", count)
	}
	if err := scheduler.BackupCheck(now.Add(24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("backups before interval = %d, %v", len(entries), err)
	}
	if err := scheduler.BackupCheck(now.Add(49 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("backups after rotation = %d, %v", len(entries), err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("backup older than 48 hours still exists: %v", err)
	}
	if err := scheduler.BackupCheck(now.Add(73 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("expired backup still exists: %v", err)
	}
}
