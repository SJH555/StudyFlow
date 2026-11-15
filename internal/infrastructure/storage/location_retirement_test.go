//go:build (linux && !android) || windows

package storage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func prepareRetirement(t *testing.T, root string) (locationFixture, *LocationSwitch) {
	t.Helper()
	f := locationFixtureAt(root)
	writeLocationFixture(t, f.oldDB, "old database")
	writeLocationFixture(t, f.targetDB, "target database")
	writeLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
	for _, base := range []string{filepath.Dir(f.oldDB), filepath.Dir(f.targetDB)} {
		writeLocationFixture(t, filepath.Join(base, "backups", "copied.db"), "copied backup")
	}
	writeLocationFixture(t, filepath.Join(filepath.Dir(f.oldDB), "backups", "notes.txt"), "user file")
	s := f.open(t)
	s.PlanRetirement([]string{filepath.Join(filepath.Dir(f.oldDB), "backups", "copied.db")})
	f.stage(t, s)
	if result, err := s.Publish(); err != nil || result.Outcome != LocationCommitted {
		t.Fatalf("publish = %+v, %v", result, err)
	}
	return f, s
}

func assertRetiredMissing(t *testing.T, f locationFixture) {
	t.Helper()
	for _, path := range []string{f.oldDB, filepath.Join(filepath.Dir(f.oldDB), "backups", "copied.db")} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retired path remains: %s: %v", path, err)
		}
	}
	assertLocationFixture(t, filepath.Join(filepath.Dir(f.oldDB), "backups", "notes.txt"), "user file")
	assertLocationFixture(t, filepath.Join(filepath.Dir(f.targetDB), "backups", "copied.db"), "copied backup")
	assertLocationFixture(t, f.targetDB, "target database")
}

func TestRetirementCleanupRetriesWithoutDeletingUserFiles(t *testing.T) {
	for _, failure := range []string{"backup unlink", "database unlink", "directory sync", "empty directory removed"} {
		t.Run(failure, func(t *testing.T) {
			f, s := prepareRetirement(t, t.TempDir())
			injected := errors.New(failure)
			if failure == "directory sync" {
				s.io.syncDir = func(*os.File) error { return injected }
			} else if failure != "empty directory removed" {
				s.io.unlinkOwned = func(fd int, name string, flags int) error {
					if (failure == "database unlink" && strings.HasSuffix(name, "-study.db")) ||
						(failure == "backup unlink" && strings.HasSuffix(name, "-copied.db")) {
						return injected
					}
					return defaultLocationIO().unlinkOwned(fd, name, flags)
				}
			} else {
				if err := os.Remove(filepath.Join(filepath.Dir(f.oldDB), "backups", "notes.txt")); err != nil {
					t.Fatal(err)
				}
			}
			err := s.CleanupRetired()
			if failure != "empty directory removed" && (!errors.Is(err, ErrRetirementCleanup) || !errors.Is(err, injected)) {
				t.Fatalf("cleanup = %v", err)
			}
			if failure == "empty directory removed" && err != nil {
				t.Fatal(err)
			}
			// 不清除标记，模拟删除已完成、标记尚未清除时重启。
			reopened := f.open(t)
			if result, err := reopened.Recover(); err != nil || result.Outcome != LocationCommitted {
				t.Fatalf("recovery = %+v, %v", result, err)
			}
			if err := reopened.CleanupRetired(); err != nil {
				t.Fatal(err)
			}
			if err := reopened.Clear(); err != nil {
				t.Fatal(err)
			}
			if failure == "empty directory removed" {
				writeLocationFixture(t, filepath.Join(filepath.Dir(f.oldDB), "backups", "notes.txt"), "user file")
			}
			assertRetiredMissing(t, f)
			assertNoLocationMarker(t, f)
		})
	}
}

func TestRetirementPreservesChangedFilesAndSQLiteSidecars(t *testing.T) {
	for _, change := range []string{"backup content", "backup identity", "target backup content", "target backup identity", "database identity", "backup directory identity", "WAL", "location changed"} {
		t.Run(change, func(t *testing.T) {
			f, s := prepareRetirement(t, t.TempDir())
			backup := filepath.Join(filepath.Dir(f.oldDB), "backups", "copied.db")
			switch change {
			case "target backup content":
				writeLocationFixture(t, filepath.Join(filepath.Dir(f.targetDB), "backups", "copied.db"), "modified copy")
			case "target backup identity":
				path := filepath.Join(filepath.Dir(f.targetDB), "backups", "copied.db")
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal(err)
				}
				writeLocationFixture(t, path, "copied backup")
			case "backup content":
				writeLocationFixture(t, backup, "modified backup")
			case "backup identity", "database identity":
				path := backup
				if change == "database identity" {
					path = f.oldDB
				}
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal(err)
				}
				writeLocationFixture(t, path, "replacement")
			case "backup directory identity":
				path := filepath.Dir(backup)
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal(err)
				}
				writeLocationFixture(t, backup, "replacement")
			case "WAL":
				writeLocationFixture(t, f.oldDB+"-wal", "retained WAL")
			case "location changed":
				writeLocationFixture(t, f.official, "external location\n")
			}
			want := ErrRetirementCleanup
			if change == "location changed" {
				want = errRetirementLocationChanged
			}
			if err := s.CleanupRetired(); !errors.Is(err, want) {
				t.Fatalf("cleanup accepted changed resource: %v", err)
			}
			if _, err := os.Stat(f.oldDB); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(f.official + ".pending"); err != nil {
				t.Fatal(err)
			}
			assertLocationFixture(t, f.targetDB, "target database")
		})
	}
}

func TestRetirementRejectsBackupChangedAfterCopy(t *testing.T) {
	f := newLocationFixture(t, "content")
	backup := filepath.Join(filepath.Dir(f.oldDB), "backups", "copied.db")
	writeLocationFixture(t, backup, "changed source")
	writeLocationFixture(t, filepath.Join(filepath.Dir(f.targetDB), "backups", "copied.db"), "copied version")
	s := f.open(t)
	s.PlanRetirement([]string{backup})
	if err := s.Stage(f.oldDB, f.targetDB, f.root); err == nil || s.HasPending() {
		t.Fatalf("stage accepted inconsistent backup: %v", err)
	}
	assertLocationFixture(t, f.oldDB, "old database")
	assertLocationFixture(t, backup, "changed source")
}

func TestRetirementCrashRecovery(t *testing.T) {
	const exitCode = 74
	if phase := os.Getenv("STUDYFLOW_RETIRE_CRASH"); phase != "" {
		_, s := prepareRetirement(t, os.Getenv("STUDYFLOW_RETIRE_ROOT"))
		s.io.unlinkOwned = func(fd int, name string, flags int) error {
			if strings.HasSuffix(name, "-study.db") {
				if phase == "after unlink" {
					if err := defaultLocationIO().unlinkOwned(fd, name, flags); err != nil {
						t.Fatal(err)
					}
				}
				os.Exit(exitCode)
			}
			return defaultLocationIO().unlinkOwned(fd, name, flags)
		}
		if err := s.CleanupRetired(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("crash hook did not run")
	}
	for _, phase := range []string{"after isolation", "after unlink"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestRetirementCrashRecovery$")
			cmd.Env = append(os.Environ(), "STUDYFLOW_RETIRE_CRASH="+phase, "STUDYFLOW_RETIRE_ROOT="+root)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != exitCode {
				t.Fatalf("crash: %v, %s", err, output)
			}
			f := locationFixtureAt(root)
			s := f.open(t)
			if result, err := s.Recover(); err != nil || result.Outcome != LocationCommitted {
				t.Fatalf("recover: %+v, %v", result, err)
			}
			if err := s.CleanupRetired(); err != nil {
				t.Fatal(err)
			}
			if err := s.Clear(); err != nil {
				t.Fatal(err)
			}
			assertRetiredMissing(t, f)
			assertNoLocationMarker(t, f)
		})
	}
}
