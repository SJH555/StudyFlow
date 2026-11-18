//go:build (linux && !android) || windows

package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/storage"
)

func stage5DirectoryPaths(root string) startupDatabasePaths {
	return startupDatabasePaths{locationFile: filepath.Join(root, "base", "storage-location.txt"), defaultDir: filepath.Join(root, "base", "data"), installDir: filepath.Join(root, "install"), legacyConfigDir: filepath.Join(root, "legacy")}
}

func TestStage5DirectoryCrashChild(t *testing.T) {
	root := os.Getenv("STUDYFLOW_STAGE5_DIRECTORY_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	p := stage5DirectoryPaths(root)
	oldPath := filepath.Join(p.defaultDir, "study.db")
	target := filepath.Join(root, "target", "study.db")
	old, err := persistence.OpenExisting(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := persistence.OpenExisting(target)
	if err != nil {
		t.Fatal(err)
	}
	_ = candidate // Deliberately held open until forced termination.
	location, err := storage.OpenLocationSwitch(p.locationFile)
	if err != nil {
		t.Fatal(err)
	}
	location.PlanRetirement(nil)
	if err := location.Stage(oldPath, target, p.installDir); err != nil {
		t.Fatal(err)
	}
	mode := os.Getenv("STUDYFLOW_STAGE5_DIRECTORY_MODE")
	if mode != "staged" {
		result, err := location.Publish()
		if err != nil || result.Outcome != storage.LocationCommitted {
			t.Fatalf("publish=%+v, %v", result, err)
		}
	}
	if mode == "cleaned" {
		if err := old.Close(); err != nil {
			t.Fatal(err)
		}
		if err := location.CleanupRetired(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestStage5ForcedTerminationRecoversStorageLocation(t *testing.T) {
	for _, mode := range []string{"staged", "published", "cleaned"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			p := stage5DirectoryPaths(root)
			oldPath := filepath.Join(p.defaultDir, "study.db")
			target := filepath.Join(root, "target", "study.db")
			seedStartupSQLite(t, oldPath, true, `INSERT INTO settings VALUES('selected','old')`)
			seedStartupSQLite(t, target, true, `INSERT INTO settings VALUES('selected','target')`)
			user := filepath.Join(p.defaultDir, "notes.txt")
			writeStartupFixture(t, user, "user file survives recovery")
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(executable, "-test.run=^TestStage5DirectoryCrashChild$", "-test.timeout=60s")
			cmd.Env = append(os.Environ(), "STUDYFLOW_STAGE5_DIRECTORY_ROOT="+root, "STUDYFLOW_STAGE5_DIRECTORY_MODE="+mode)
			output, err := os.Create(filepath.Join(root, "child-output.txt"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			cmd.Stdout = output
			cmd.Stderr = output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					<-done
				}
			}()
			deadline := time.NewTimer(15 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
		waiting:
			for {
				select {
				case err := <-done:
					waited = true
					data, _ := os.ReadFile(filepath.Join(root, "child-output.txt"))
					t.Fatalf("child exited before ready: %v, %s", err, data)
				case <-deadline.C:
					t.Fatal("child readiness timed out")
				case <-ticker.C:
					if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
						break waiting
					}
				}
			}
			if _, err := os.Stat(p.locationFile + ".pending"); err != nil {
				t.Fatal("no pending record before termination", err)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = <-done
			waited = true
			if err == nil {
				t.Fatal("child terminated normally")
			}
			recovered, err := openStartupDatabase(p, persistence.OpenForStartup)
			if err != nil {
				t.Fatal(err)
			}
			defer recovered.Close()
			wantPath, wantValue := target, "target"
			if mode == "staged" {
				wantPath, wantValue = oldPath, "old"
			}
			if recovered.Path() != wantPath {
				t.Fatalf("selected %s, want %s", recovered.Path(), wantPath)
			}
			if err := recovered.WithRead(func(db *sql.DB, _ string) error {
				var value, integrity string
				if err := db.QueryRow(`SELECT value FROM settings WHERE key='selected'`).Scan(&value); err != nil {
					return err
				}
				if value != wantValue {
					t.Fatalf("selected data=%s, want %s", value, wantValue)
				}
				if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
					return err
				}
				if integrity != "ok" {
					t.Fatal(integrity)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			assertStartupPathMissing(t, p.locationFile+".pending")
			assertStartupFileUnchanged(t, user, []byte("user file survives recovery"))
			// A second startup must select the same database without another recovery.
			if err := recovered.Close(); err != nil {
				t.Fatal(err)
			}
			again, err := openStartupDatabase(p, persistence.OpenForStartup)
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			if again.Path() != wantPath {
				t.Fatalf("second startup selected %s", again.Path())
			}
		})
	}
}
