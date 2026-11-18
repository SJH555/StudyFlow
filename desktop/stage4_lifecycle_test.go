package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func TestStage4BackupWaitsForTransactionAndCopiesCommittedData(t *testing.T) {
	for _, commit := range []bool{true, false} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			api := newAPITestService(t)
			businessFixture(t, api, `INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1') ON CONFLICT(key) DO UPDATE SET value='1'`)
			started, release := make(chan struct{}), make(chan struct{})
			txDone := make(chan error, 1)
			failure := errors.New("rejected")
			go func() {
				txDone <- api.app.database.WithAccess(context.Background(), func(a *persistence.Access) error {
					return a.Queries(context.Background()).WithTx(func(q *persistence.Queries) error {
						if _, err := q.Exec(`INSERT INTO settings VALUES('stage4-write','saved')`); err != nil {
							return err
						}
						close(started)
						<-release
						if !commit {
							return failure
						}
						return nil
					})
				})
			}()
			<-started
			backupDone := make(chan error, 1)
			now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.Local)
			go func() { backupDone <- api.app.scheduler.BackupCheck(now) }()
			select {
			case err := <-backupDone:
				t.Fatalf("backup preceded cleanup: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			err := <-txDone
			if (commit && err != nil) || (!commit && !errors.Is(err, failure)) {
				t.Fatalf("transaction=%v", err)
			}
			if err := <-backupDone; err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(filepath.Dir(api.app.database.Path()), "backups", "study-20261118-160000.db")
			copyDB, err := persistence.OpenExisting(path)
			if err != nil {
				t.Fatal(err)
			}
			defer copyDB.Close()
			var count int
			if err := copyDB.QueryRow(`SELECT count(*) FROM settings WHERE key='stage4-write'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 0
			if commit {
				want = 1
			}
			if count != want {
				t.Fatalf("backup has %d writes, want %d", count, want)
			}
			var journal string
			if err := copyDB.QueryRow(`PRAGMA integrity_check`).Scan(&journal); err != nil || journal != "ok" {
				t.Fatalf("backup integrity=%s, %v", journal, err)
			}
		})
	}
}

func TestStage4ShutdownWaitsForCancelledTransactionRollback(t *testing.T) {
	api := newAPITestService(t)
	path := api.app.database.Path()
	entered, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	api.app.tasks.start(func(ctx context.Context) {
		result <- api.app.database.WithAccess(ctx, func(a *persistence.Access) error {
			return a.Queries(ctx).WithTx(func(q *persistence.Queries) error {
				if _, err := q.Exec(`INSERT INTO settings VALUES('uncommitted-exit','must disappear')`); err != nil {
					return err
				}
				close(entered)
				<-ctx.Done()
				<-release
				return nil
			})
		})
	})
	<-entered
	closed := make(chan error, 1)
	go func() { api.app.tasks.stopAndWait(); closed <- api.app.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("shutdown preceded rollback: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if _, _, ok := api.app.tasks.enter(); ok {
		t.Fatal("shutdown accepted new work")
	}
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("transaction=%v", err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := api.app.database.WithAccess(context.Background(), func(*persistence.Access) error { t.Error("closed access ran"); return nil }); !errors.Is(err, persistence.ErrDatabaseClosed) {
		t.Fatalf("closed access=%v", err)
	}
	db, err := persistence.OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM settings WHERE key='uncommitted-exit'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("exit left partial write=%d, %v", count, err)
	}
}

func TestStage4CancelledBackupDoesNotCleanExpiredFiles(t *testing.T) {
	api := newAPITestService(t)
	path := api.app.database.Path()
	old := filepath.Join(filepath.Dir(path), "backups", "study-20200101-000000.db")
	if err := os.MkdirAll(filepath.Dir(old), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("previous backup"), 0600); err != nil {
		t.Fatal(err)
	}
	businessFixture(t, api, `INSERT INTO settings(key,value) VALUES('autoBackupEnabled','1') ON CONFLICT(key) DO UPDATE SET value='1'`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := api.app.scheduler.WithContext(ctx).BackupCheck(time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("backup cancellation=%v", err)
	}
	// Also exercise cancellation after obtaining the raw handle, not just the
	// scheduler's early Context check.
	err := api.app.database.WithWrite(func(db *sql.DB, _ string) error { return persistence.CreateBackupContext(ctx, db, path, time.Now()) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot cancellation=%v", err)
	}
	if err := persistence.RemoveExpiredBackupsContext(ctx, path, []persistence.BackupFile{{Name: filepath.Base(old), Timestamp: time.Time{}}}, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup cancellation=%v", err)
	}
	data, err := os.ReadFile(old)
	if err != nil || string(data) != "previous backup" {
		t.Fatalf("previous backup changed: %s, %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(old))
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected backup entries=%v, %v", entries, err)
	}
}
