//go:build (linux && !android) || windows

package storage

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type locationFixture struct{ root, official, oldDB, targetDB string }

func locationFixtureAt(root string) locationFixture {
	return locationFixture{root, filepath.Join(root, "storage-location.txt"),
		filepath.Join(root, "old", "study.db"), filepath.Join(root, "target", "study.db")}
}

func newLocationFixture(t *testing.T, oldState string) locationFixture {
	t.Helper()
	f := locationFixtureAt(t.TempDir())
	writeLocationFixture(t, f.oldDB, "old database")
	writeLocationFixture(t, f.targetDB, "target database")
	switch oldState {
	case "empty":
		writeLocationFixture(t, f.official, "")
	case "content":
		writeLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
	}
	return f
}

func writeLocationFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func (f locationFixture) open(t *testing.T) *LocationSwitch {
	t.Helper()
	s, err := OpenLocationSwitch(f.official)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func (f locationFixture) stage(t *testing.T, s *LocationSwitch) {
	t.Helper()
	if err := s.Stage(f.oldDB, f.targetDB, f.root); err != nil {
		t.Fatal(err)
	}
}

func assertLocationFixture(t *testing.T, path, want string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Fatalf("%s = %q, %v; want %q", path, data, err, want)
	}
}

func assertNoLocationMarker(t *testing.T, f locationFixture) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(f.root, pendingLocationName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending location should be absent: %v", err)
	}
}

func TestLocationSwitchPreservesOldLocationStates(t *testing.T) {
	for _, state := range []string{"missing", "empty", "content"} {
		t.Run(state, func(t *testing.T) {
			f := newLocationFixture(t, state)
			s := f.open(t)
			f.stage(t, s)
			if s.record.OldLocation.State != state || s.record.OldLocation.ID != s.old.ID {
				t.Fatalf("old state was not preserved: %+v", s.record.OldLocation)
			}
			reopened := f.open(t)
			result, err := reopened.Recover()
			if err != nil || result != (LocationResolution{LocationUnchanged, f.oldDB}) {
				t.Fatalf("recovery = %+v, %v", result, err)
			}
			result, err = s.Publish()
			if err != nil || result != (LocationResolution{LocationCommitted, f.targetDB}) {
				t.Fatalf("publication = %+v, %v", result, err)
			}
			assertLocationFixture(t, f.official, filepath.Dir(f.targetDB)+"\n")
			if err := s.Clear(); err != nil {
				t.Fatal(err)
			}
			assertNoLocationMarker(t, f)
			assertLocationFixture(t, f.oldDB, "old database")
			assertLocationFixture(t, f.targetDB, "target database")
		})
	}
}

func TestLocationSwitchRequiresDurablePendingRecord(t *testing.T) {
	for _, failure := range []string{"marker write", "marker file sync", "marker directory sync", "position file close", "marker file close"} {
		t.Run(failure, func(t *testing.T) {
			f := newLocationFixture(t, "content")
			s := f.open(t)
			injected := errors.New(failure)
			switch failure {
			case "marker write":
				s.io.write = func(file *os.File, data []byte) (int, error) {
					if strings.HasSuffix(file.Name(), "-pending") {
						return file.Write(data[:1])
					}
					return file.Write(data)
				}
				injected = io.ErrShortWrite
			case "marker file sync":
				s.io.syncFile = func(file *os.File) error {
					if strings.HasSuffix(file.Name(), "-pending") {
						return injected
					}
					return file.Sync()
				}
			case "marker directory sync":
				s.io.syncDir = func(*os.File) error { return injected }
			case "position file close", "marker file close":
				s.io.closeFile = func(file *os.File) error {
					err := file.Close()
					if strings.HasSuffix(file.Name(), "-pending") == (failure == "marker file close") {
						return errors.Join(err, injected)
					}
					return err
				}
			}
			if err := s.Stage(f.oldDB, f.targetDB, f.root); !errors.Is(err, injected) {
				t.Fatalf("stage error = %v", err)
			}
			if s.HasPending() != (failure == "marker directory sync") {
				t.Fatalf("pending evidence = %t", s.HasPending())
			}
			if result, err := s.Publish(); err == nil || result.Outcome != LocationUncertain {
				t.Fatalf("published without durable record: %+v, %v", result, err)
			}
			assertLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
			if !s.HasPending() {
				assertNoLocationMarker(t, f)
				if files, err := filepath.Glob(filepath.Join(f.root, locationTempPrefix+"*")); err != nil || len(files) != 0 {
					t.Fatalf("failed stage left temporary files: %v, %v", files, err)
				}
			}
		})
	}
}

func TestLocationSwitchPublicationEvidence(t *testing.T) {
	for _, failure := range []string{"rename failed", "rename reported error after success", "readback failed", "identity changed", "content changed", "directory sync failed", "external preflight edit", "target identity changed"} {
		t.Run(failure, func(t *testing.T) {
			f := newLocationFixture(t, "content")
			s := f.open(t)
			f.stage(t, s)
			injected := errors.New(failure)
			var attempts int
			s.io.rename = func(dir int, old string, targetDir int, target string, flags uint) error {
				attempts++
				if failure == "rename failed" {
					return injected
				}
				if err := defaultLocationIO().rename(dir, old, targetDir, target, flags); err != nil {
					return err
				}
				switch failure {
				case "rename reported error after success":
					return injected
				case "readback failed", "identity changed":
					if err := os.Rename(f.official, f.official+".held"); err != nil {
						t.Fatal(err)
					}
					if failure == "readback failed" {
						if err := os.Mkdir(f.official, 0700); err != nil {
							t.Fatal(err)
						}
					} else {
						writeLocationFixture(t, f.official, filepath.Dir(f.targetDB)+"\n")
					}
				case "content changed":
					writeLocationFixture(t, f.official, "external content")
				}
				return nil
			}
			switch failure {
			case "directory sync failed":
				s.io.syncDir = func(*os.File) error { return injected }
			case "external preflight edit":
				writeLocationFixture(t, f.official, "external content")
			case "target identity changed":
				if err := os.Rename(f.targetDB, f.targetDB+".held"); err != nil {
					t.Fatal(err)
				}
				writeLocationFixture(t, f.targetDB, "replacement database")
			}
			result, err := s.Publish()
			want := LocationUncertain
			switch failure {
			case "rename failed":
				want = LocationUnchanged
				assertLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
			case "rename reported error after success":
				want = LocationCommitted
			case "external preflight edit", "target identity changed":
				if attempts != 0 {
					t.Fatal("preflight failure did not prevent rename")
				}
			}
			if result.Outcome != want || err == nil {
				t.Fatalf("publication = %+v, %v; want %v with error", result, err, want)
			}
			if want == LocationUncertain {
				if !s.HasPending() || s.Clear() == nil {
					t.Fatal("uncertain publication must retain its pending record")
				}
			} else if !errors.Is(err, injected) || s.Clear() != nil {
				t.Fatalf("expected preserved publication error and clearable record: %v", err)
			}
			assertLocationFixture(t, f.oldDB, "old database")
		})
	}
}

func TestLocationSwitchClearRetriesDirectorySync(t *testing.T) {
	f := newLocationFixture(t, "content")
	s := f.open(t)
	f.stage(t, s)
	if result, err := s.Publish(); result.Outcome != LocationCommitted || err != nil {
		t.Fatalf("publication = %+v, %v", result, err)
	}
	injected := errors.New("marker removal sync failed")
	s.io.syncDir = func(*os.File) error { return injected }
	if err := s.Clear(); !errors.Is(err, injected) || !s.HasPending() {
		t.Fatalf("clear = %v; pending = %t", err, s.HasPending())
	}
	assertNoLocationMarker(t, f)
	if err := s.Stage(f.oldDB, f.targetDB, f.root); err == nil {
		t.Fatal("new stage allowed before marker removal was synchronized")
	}
	s.io.syncDir = defaultLocationIO().syncDir
	if err := s.Clear(); err != nil || s.HasPending() {
		t.Fatalf("clear retry = %v; pending = %t", err, s.HasPending())
	}
	assertLocationFixture(t, f.official, filepath.Dir(f.targetDB)+"\n")
}

func TestLocationSwitchRetriesRecoveredTemporaryFileCleanup(t *testing.T) {
	for _, failure := range []string{"unlink", "directory sync", "interrupted isolation"} {
		t.Run(failure, func(t *testing.T) {
			f := newLocationFixture(t, "content")
			original := f.open(t)
			f.stage(t, original)
			temp := original.record.TempName
			marker := filepath.Join(f.root, pendingLocationName)
			tempPath := filepath.Join(f.root, temp)
			isolatedPath := tempPath + locationCleanupSuffix
			if failure == "interrupted isolation" {
				if err := os.Rename(tempPath, isolatedPath); err != nil {
					t.Fatal(err)
				}
			} else {
				reopened := f.open(t)
				if result, err := reopened.Recover(); result.Outcome != LocationUnchanged || err != nil {
					t.Fatalf("recovery = %+v, %v", result, err)
				}
				injected := errors.New("temporary location cleanup failed")
				if failure == "unlink" {
					reopened.io.unlinkOwned = func(fd int, name string, flags int) error {
						if name == temp+locationCleanupSuffix {
							return injected
						}
						return defaultLocationIO().unlinkOwned(fd, name, flags)
					}
				} else {
					reopened.io.syncDir = func(*os.File) error { return injected }
				}
				if err := reopened.Clear(); !errors.Is(err, injected) || !reopened.HasPending() {
					t.Fatalf("clear = %v; pending = %t", err, reopened.HasPending())
				}
				if failure == "unlink" {
					assertLocationFixture(t, tempPath, filepath.Dir(f.targetDB)+"\n")
				} else if _, err := os.Lstat(tempPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("temporary file should be removed before sync: %v", err)
				}
			}
			assertLocationFixture(t, marker, string(original.marker.Bytes))
			retry := f.open(t)
			if result, err := retry.Recover(); result.Outcome != LocationUnchanged || err != nil {
				t.Fatalf("retry recovery = %+v, %v", result, err)
			}
			if err := retry.Clear(); err != nil || retry.HasPending() {
				t.Fatalf("clear retry = %v; pending = %t", err, retry.HasPending())
			}
			assertNoLocationMarker(t, f)
			for _, path := range []string{tempPath, isolatedPath} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("%s should be absent after retry: %v", path, err)
				}
			}
			assertLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
			assertLocationFixture(t, f.targetDB, "target database")
		})
	}
}

func TestLocationSwitchRecoversIsolatedPendingRecord(t *testing.T) {
	f := newLocationFixture(t, "content")
	s := f.open(t)
	f.stage(t, s)
	if result, err := s.Publish(); result.Outcome != LocationCommitted || err != nil {
		t.Fatalf("publication = %+v, %v", result, err)
	}
	marker := filepath.Join(f.root, pendingLocationName)
	isolated := marker + locationCleanupSuffix
	if err := os.Rename(marker, isolated); err != nil {
		t.Fatal(err)
	}
	reopened := f.open(t)
	if result, err := reopened.Recover(); result.Outcome != LocationCommitted || err != nil {
		t.Fatalf("recovery = %+v, %v", result, err)
	}
	if err := reopened.Clear(); err != nil || reopened.HasPending() {
		t.Fatalf("clear = %v; pending = %t", err, reopened.HasPending())
	}
	assertNoLocationMarker(t, f)
	if _, err := os.Lstat(isolated); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated marker should be absent: %v", err)
	}
	assertLocationFixture(t, f.official, filepath.Dir(f.targetDB)+"\n")
	assertLocationFixture(t, f.oldDB, "old database")
}

func TestLocationSwitchKeepsFileReplacedBeforeIsolation(t *testing.T) {
	f := newLocationFixture(t, "content")
	s := f.open(t)
	f.stage(t, s)
	reopened := f.open(t)
	if result, err := reopened.Recover(); result.Outcome != LocationUnchanged || err != nil {
		t.Fatalf("recovery = %+v, %v", result, err)
	}
	temp := reopened.record.TempName
	path := filepath.Join(f.root, temp)
	var replacement locationState
	reopened.io.moveOwned = func(fd int, name string, toFD int, to string, flags uint) error {
		if name == temp {
			if err := os.Rename(path, path+".held"); err != nil {
				t.Fatal(err)
			}
			writeLocationFixture(t, path, filepath.Dir(f.targetDB)+"\n")
			var err error
			replacement, err = reopened.read(temp, maxLocationBytes)
			if err != nil {
				t.Fatal(err)
			}
		}
		return defaultLocationIO().rename(fd, name, toFD, to, flags)
	}
	reopened.io.unlinkOwned = func(int, string, int) error {
		t.Fatal("ownership mismatch must prevent unlink")
		return nil
	}
	if err := reopened.Clear(); err == nil || !reopened.HasPending() {
		t.Fatalf("clear = %v; pending = %t", err, reopened.HasPending())
	}
	current, err := reopened.read(temp, maxLocationBytes)
	if err != nil || !sameLocationState(current, replacement) || replacement.ID == reopened.record.NewLocation.ID {
		t.Fatalf("replacement was not preserved at its original path: %v, %v", current, err)
	}
	assertLocationFixture(t, path+".held", filepath.Dir(f.targetDB)+"\n")
	assertLocationFixture(t, filepath.Join(f.root, pendingLocationName), string(s.marker.Bytes))
}

func TestLocationSwitchRejectsInvalidPendingRecords(t *testing.T) {
	for _, invalid := range []string{"unknown version", "malformed JSON", "directory identity mismatch", "marker identity mismatch"} {
		t.Run(invalid, func(t *testing.T) {
			f := newLocationFixture(t, "content")
			s := f.open(t)
			f.stage(t, s)
			path := filepath.Join(f.root, pendingLocationName)
			var data string
			switch invalid {
			case "malformed JSON":
				data = "{"
			case "marker identity mismatch":
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal(err)
				}
				data = string(s.marker.Bytes)
			default:
				record := *s.record
				if invalid == "unknown version" {
					record.Version++
				} else {
					record.PositionDirectoryID.Inode++
				}
				encoded, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				data = string(encoded)
			}
			writeLocationFixture(t, path, data)
			if invalid == "marker identity mismatch" {
				if result, err := s.Recover(); err == nil || result.Outcome != LocationUncertain || s.Clear() == nil {
					t.Fatalf("recovery accepted an externally replaced marker: %+v, %v", result, err)
				}
				assertLocationFixture(t, path+".held", data)
			} else if reopened, err := OpenLocationSwitch(f.official); err == nil {
				_ = reopened.Close()
				t.Fatal("opened an invalid pending record")
			}
			assertLocationFixture(t, path, data)
			assertLocationFixture(t, filepath.Join(f.root, s.record.TempName), filepath.Dir(f.targetDB)+"\n")
			assertLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
			assertLocationFixture(t, f.oldDB, "old database")
			assertLocationFixture(t, f.targetDB, "target database")
		})
	}
}

func TestLocationSwitchDoesNotOverwriteConcurrentCreation(t *testing.T) {
	f := newLocationFixture(t, "missing")
	s := f.open(t)
	f.stage(t, s)
	s.io.rename = func(dir int, old string, targetDir int, target string, flags uint) error {
		writeLocationFixture(t, f.official, "another writer")
		return defaultLocationIO().rename(dir, old, targetDir, target, flags)
	}
	if result, err := s.Publish(); result.Outcome != LocationUncertain || !errors.Is(err, os.ErrExist) {
		t.Fatalf("publication = %+v, %v", result, err)
	}
	assertLocationFixture(t, f.official, "another writer")
	if !s.HasPending() || s.Clear() == nil {
		t.Fatal("the unresolved switch must retain its record")
	}
}

func TestLocationSwitchRecoveryRejectsReplacedDatabase(t *testing.T) {
	f := newLocationFixture(t, "content")
	s := f.open(t)
	f.stage(t, s)
	if result, err := s.Publish(); result.Outcome != LocationCommitted || err != nil {
		t.Fatalf("publication = %+v, %v", result, err)
	}
	if err := os.Rename(f.targetDB, f.targetDB+".held"); err != nil {
		t.Fatal(err)
	}
	writeLocationFixture(t, f.targetDB, "target database")
	reopened := f.open(t)
	if result, err := reopened.Recover(); result.Outcome != LocationUncertain || err == nil {
		t.Fatalf("recovery = %+v, %v", result, err)
	}
	if err := reopened.Stage(f.oldDB, f.targetDB, f.root); err == nil {
		t.Fatal("recovery must not replace its existing record")
	}
	if !reopened.HasPending() || reopened.Clear() == nil {
		t.Fatal("recovery must retain the unresolved record")
	}
}

func TestLocationSwitchRejectsUnsafePaths(t *testing.T) {
	f := newLocationFixture(t, "missing")
	for _, path := range []string{f.root, f.root + " ", f.root + "\nnew", f.root + "/\xff"} {
		if err := CheckSwitchTarget(path, f.root); err == nil {
			t.Fatalf("accepted unrepresentable or install-marker path %q", path)
		}
	}
	if err := CheckSwitchTarget(filepath.Join(f.root, "inner space"), f.root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Base(f.official), pendingLocationName} {
		path := filepath.Join(f.root, name)
		if err := os.Symlink(f.oldDB, path); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if s, err := OpenLocationSwitch(f.official); err == nil {
			_ = s.Close()
			t.Fatalf("accepted location symlink %s", path)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocationSwitchCrashRecovery(t *testing.T) {
	const crashExit = 73
	if phase := os.Getenv("STUDYFLOW_LOCATION_CRASH_PHASE"); phase != "" {
		f := locationFixtureAt(os.Getenv("STUDYFLOW_LOCATION_CRASH_ROOT"))
		s := f.open(t)
		if phase == "writing marker" {
			s.io.write = func(file *os.File, data []byte) (int, error) {
				if strings.HasSuffix(file.Name(), "-pending") {
					if _, err := file.Write(data[:1]); err != nil {
						t.Fatal(err)
					}
					os.Exit(crashExit)
				}
				return file.Write(data)
			}
		}
		f.stage(t, s)
		switch phase {
		case "marker durable":
		case "location renamed", "location directory synced":
			if err := s.rename(s.record.TempName, s.name, 0); err != nil {
				t.Fatal(err)
			}
			if phase == "location directory synced" {
				if err := s.syncDirectory(); err != nil {
					t.Fatal(err)
				}
			}
		case "after memory commit", "before marker removal":
			// 这两个阶段具有相同的磁盘状态。数据库所有权的测试将在切换入口与该协议集成后进行。
			if result, err := s.Publish(); result.Outcome != LocationCommitted || err != nil {
				t.Fatalf("publication = %+v, %v", result, err)
			}
		default:
			t.Fatalf("unexpected crash phase %q", phase)
		}
		os.Exit(crashExit)
	}
	for _, phase := range []string{"writing marker", "marker durable", "location renamed", "location directory synced", "after memory commit", "before marker removal"} {
		t.Run(phase, func(t *testing.T) {
			f := newLocationFixture(t, "content")
			cmd := exec.Command(os.Args[0], "-test.run=^TestLocationSwitchCrashRecovery$")
			cmd.Env = append(os.Environ(), "STUDYFLOW_LOCATION_CRASH_PHASE="+phase, "STUDYFLOW_LOCATION_CRASH_ROOT="+f.root)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != crashExit {
				t.Fatalf("subprocess did not exit at crash point: %v, %s", err, output)
			}
			s := f.open(t)
			switch phase {
			case "writing marker":
				if s.HasPending() {
					t.Fatal("partial marker should not be visible under its official name")
				}
				assertLocationFixture(t, f.official, filepath.Dir(f.oldDB)+"\n")
			default:
				want := LocationResolution{LocationCommitted, f.targetDB}
				temp := filepath.Join(f.root, s.record.TempName)
				if phase == "marker durable" {
					want = LocationResolution{LocationUnchanged, f.oldDB}
					assertLocationFixture(t, temp, filepath.Dir(f.targetDB)+"\n")
				}
				if result, err := s.Recover(); result != want || err != nil {
					t.Fatalf("recovery = %+v, %v; want %+v", result, err, want)
				}
				if err := s.Clear(); err != nil {
					t.Fatal(err)
				}
				assertNoLocationMarker(t, f)
				if _, err := os.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("temporary location should be absent after recovery: %v", err)
				}
			}
			assertLocationFixture(t, f.oldDB, "old database")
			assertLocationFixture(t, f.targetDB, "target database")
		})
	}
}
