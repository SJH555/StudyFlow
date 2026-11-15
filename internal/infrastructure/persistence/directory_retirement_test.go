//go:build (linux && !android) || windows

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"exam_notify/internal/infrastructure/storage"
)

func TestSwitchDirectoryReturnsToOriginalDirectory(t *testing.T) {
	f := newSwitchFixture(t)
	user := filepath.Join(filepath.Dir(f.oldPath), "backups", "notes.txt")
	writeDirectoryFixture(t, user, "keep user file")
	for _, target := range []string{f.target, filepath.Dir(f.oldPath)} {
		result, err := f.db.SwitchDirectory(context.Background(), target, f.location, f.root)
		if err != nil || !result.Committed || result.Warning != nil {
			t.Fatalf("switch to %s: %+v, %v", target, result, err)
		}
		f.assertActive(t, filepath.Join(target, "study.db"))
	}
	assertDirectoryMissing(t, f.newPath)
	assertDirectoryFixture(t, user, "keep user file")
	assertDirectoryFixture(t, filepath.Join(filepath.Dir(f.oldPath), "backups", "existing.db"), "old backup")
	assertDirectoryMissing(t, f.pendingPath())
}

func TestSwitchDirectoryRecoveryAfterCleanupBeforeMarkerClear(t *testing.T) {
	f := newSwitchFixture(t)
	ops := defaultDirectorySwitchOps()
	injected := errors.New("stop before clearing marker")
	ops.clear = func(*storage.LocationSwitch) error { return injected }
	result, err := f.run(ops)
	if err != nil || !result.Committed || !errors.Is(result.Warning, injected) {
		t.Fatalf("switch = %+v, %v", result, err)
	}
	f.assertActive(t, f.newPath)
	assertDirectoryMissing(t, f.oldPath)
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	handled, err := storage.RecoverStartupLocation(f.location, func(path string, unchanged bool, clear func() error) error {
		if unchanged || path != f.newPath {
			return fmt.Errorf("wrong recovery path: %s", path)
		}
		db, err := OpenExisting(path)
		if err != nil {
			return err
		}
		defer db.Close()
		return clear()
	})
	if err != nil || !handled {
		t.Fatalf("recovery = %t, %v", handled, err)
	}
	assertDirectoryMissing(t, f.pendingPath())
}

func TestSwitchDirectoryConcurrentSwitchesAndWrites(t *testing.T) {
	f := newSwitchFixture(t)
	start := make(chan struct{})
	errs := make(chan error, 22)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs <- f.db.WithWrite(func(db *sql.DB, _ string) error {
				_, err := db.Exec("INSERT INTO settings(key,value) VALUES(?, 'written')", fmt.Sprintf("concurrent-%d", i))
				return err
			})
		}(i)
	}
	for _, target := range []string{f.target, filepath.Join(f.root, "third")} {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			<-start
			result, err := f.db.SwitchDirectory(context.Background(), target, f.location, f.root)
			if err == nil && !result.Committed {
				err = errors.New("switch was not committed")
			}
			errs <- errors.Join(err, result.Warning)
		}(target)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.db.WithRead(func(db *sql.DB, path string) error {
		var count, fk int
		if err := db.QueryRow("SELECT COUNT(*) FROM settings WHERE key LIKE 'concurrent-%'").Scan(&count); err != nil {
			return err
		}
		if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
			return err
		}
		if count != 20 || fk != 1 {
			return fmt.Errorf("rows=%d foreign_keys=%d", count, fk)
		}
		location, err := os.ReadFile(f.location)
		if err != nil {
			return err
		}
		if string(location) != filepath.Dir(path)+"\n" {
			return errors.New("published path differs from borrowed database")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertDirectoryMissing(t, f.oldPath)
	assertDirectoryMissing(t, f.pendingPath())
}
