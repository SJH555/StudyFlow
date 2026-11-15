//go:build (linux && !android) || windows

package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/storage"
)

func TestStartupCommittedCleanupFailureKeepsNewDatabaseUsable(t *testing.T) {
	p := startupPathsFixture(t)
	old := filepath.Join(p.defaultDir, "study.db")
	target := filepath.Join(filepath.Dir(p.defaultDir), "target", "study.db")
	seedStartupSQLite(t, old, true)
	seedStartupSQLite(t, target, true, `INSERT INTO settings VALUES('cleanup-new','available')`)
	location, err := storage.OpenLocationSwitch(p.locationFile)
	if err != nil {
		t.Fatal(err)
	}
	location.PlanRetirement(nil)
	if err := location.Stage(old, target, p.installDir); err != nil {
		t.Fatal(err)
	}
	if result, err := location.Publish(); err != nil || result.Outcome != storage.LocationCommitted {
		t.Fatalf("publish = %+v, %v", result, err)
	}
	if err := location.Close(); err != nil {
		t.Fatal(err)
	}
	// 在记录形成后替换旧文件；新库可用，但恢复不能删除这个替代文件。
	if err := os.Rename(old, old+".held"); err != nil {
		t.Fatal(err)
	}
	writeStartupFixture(t, old, "external replacement")
	database, err := openStartupDatabase(p, persistence.OpenExisting)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if database.Path() != target {
		t.Fatalf("active path = %s", database.Path())
	}
	if err := database.WithRead(func(db *sql.DB, _ string) error {
		var value string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key='cleanup-new'`).Scan(&value); err != nil {
			return err
		}
		if value != "available" {
			t.Fatalf("new data = %s", value)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertStartupFileUnchanged(t, old, []byte("external replacement"))
	if _, err := os.Stat(p.locationFile + ".pending"); err != nil {
		t.Fatal(err)
	}
}

type startupPendingFixture struct {
	paths                  startupDatabasePaths
	oldDB, targetDB        string
	marker, stagedLocation string
}

func makeStartupPending(t *testing.T, commit, legacyTarget bool) startupPendingFixture {
	t.Helper()
	p := startupPathsFixture(t)
	f := startupPendingFixture{
		paths:    p,
		oldDB:    filepath.Join(p.defaultDir, "study.db"),
		targetDB: filepath.Join(filepath.Dir(p.defaultDir), "target", "study.db"),
		marker:   p.locationFile + ".pending",
	}
	seedStartupSQLite(t, f.oldDB, true, `INSERT INTO settings VALUES('selected','old')`)
	if legacyTarget {
		seedHistoricalStartupDatabase(t, f.targetDB, 1)
	} else {
		seedStartupSQLite(t, f.targetDB, true, `INSERT INTO settings VALUES('selected','target')`,
			`DELETE FROM settings WHERE key='dailyReviewEnabled'`)
	}
	location, err := storage.OpenLocationSwitch(p.locationFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := location.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := location.Stage(f.oldDB, f.targetDB, p.installDir); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(p.locationFile), ".studyflow-location-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one staged location file: %v, %v", matches, err)
	}
	f.stagedLocation = matches[0]
	if commit {
		resolution, err := location.Publish()
		if err != nil || resolution.Outcome != storage.LocationCommitted {
			t.Fatalf("publish = %+v, %v", resolution, err)
		}
	}
	return f
}

// TestStartupPendingRestoresCommittedAndUnchanged 验证启动时按提交或未变更两种情形恢复 pending 记录。
func TestStartupPendingRestoresCommittedAndUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name     string
		commit   bool
		isolated bool
	}{
		{name: "committed", commit: true},
		{name: "unchanged with implicit default"},
		{name: "interrupted cleanup", commit: true, isolated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := makeStartupPending(t, tc.commit, false)
			if tc.isolated {
				if err := os.Rename(f.marker, f.marker+".cleanup"); err != nil {
					t.Fatal(err)
				}
			}
			oldBefore := readStartupFixture(t, f.oldDB)
			targetBefore := readStartupFixture(t, f.targetDB)
			wantPath, wantValue := f.oldDB, "old"
			if tc.commit {
				wantPath, wantValue = f.targetDB, "target"
			}

			var original *sql.DB
			database, err := openStartupDatabase(f.paths, func(path string) (*sql.DB, error) {
				var err error
				original, err = persistence.OpenExisting(path)
				return original, err
			})
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if database.Path() != wantPath {
				t.Fatalf("recovered path = %q, want %q", database.Path(), wantPath)
			}
			if err := database.WithRead(func(db *sql.DB, _ string) error {
				if db != original {
					t.Fatal("startup opened a second connection after recovery")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			assertStartupValue(t, database, `SELECT value FROM settings WHERE key='selected'`, wantValue)
			assertStartupPathMissing(t, f.marker)
			assertStartupPathMissing(t, f.marker+".cleanup")
			assertStartupPathMissing(t, f.stagedLocation)
			if tc.commit {
				assertStartupFileUnchanged(t, f.paths.locationFile, []byte(filepath.Dir(f.targetDB)+"\n"))
				assertStartupValue(t, database, `SELECT count(*) FROM settings WHERE key='dailyReviewEnabled'`, "0")
			} else {
				assertStartupPathMissing(t, f.paths.locationFile)
			}
			assertStartupFileUnchanged(t, f.oldDB, oldBefore)
			assertStartupFileUnchanged(t, f.targetDB, targetBefore)
		})
	}
}

// TestStartupPendingFailurePreservesEvidence 验证 pending 处理失败时保留现场证据。
func TestStartupPendingFailurePreservesEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, want   string
		legacyTarget bool
	}{
		{name: "identity conflict", want: "database identity could not be confirmed"},
		{name: "corrupt marker", want: "decode pending storage location"},
		{name: "uncertain publication", want: "official storage location does not match"},
		{name: "candidate schema failure", want: "validate recovered study database", legacyTarget: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := makeStartupPending(t, true, tc.legacyTarget)
			switch tc.name {
			case "identity conflict":
				if err := os.Rename(f.targetDB, f.targetDB+".original"); err != nil {
					t.Fatal(err)
				}
				seedStartupSQLite(t, f.targetDB, true)
			case "corrupt marker":
				writeStartupFixture(t, f.marker, "{")
			case "uncertain publication":
				// 用字节相同但身份不同的文件替换官方位置文件。
				replacement := f.paths.locationFile + ".replacement"
				writeStartupFixture(t, replacement, filepath.Dir(f.targetDB)+"\n")
				if err := os.Rename(replacement, f.paths.locationFile); err != nil {
					t.Fatal(err)
				}
			}

			before := make(map[string][]byte)
			for _, file := range []string{f.oldDB, f.targetDB, f.paths.locationFile, f.marker} {
				before[file] = readStartupFixture(t, file)
			}
			assertStartupRejected(t, f.paths, tc.want)
			for path, data := range before {
				assertStartupFileUnchanged(t, path, data)
			}
		})
	}
}

// TestStartupPendingUnchangedRechecksHistoricalSelection 验证未变更时重新校验历史选择。
func TestStartupPendingUnchangedRechecksHistoricalSelection(t *testing.T) {
	f := makeStartupPending(t, false, false)
	custom := filepath.Join(f.paths.installDir, "another")
	path := filepath.Join(custom, "study.db")
	seedStartupSQLite(t, path, true)
	writeStartupFixture(t, filepath.Join(f.paths.installDir, "storage-location.txt"), custom+"\n")
	marker := readStartupFixture(t, f.marker)
	old := readStartupFixture(t, f.oldDB)
	target := readStartupFixture(t, f.targetDB)
	assertStartupRejected(t, f.paths, "does not match startup selection")
	assertStartupPathMissing(t, f.paths.locationFile)
	assertStartupFileUnchanged(t, f.marker, marker)
	assertStartupFileUnchanged(t, f.oldDB, old)
	assertStartupFileUnchanged(t, f.targetDB, target)
}

// TestStartupPendingClearFailureClosesValidatedConnection 验证清除失败时关闭已验证的连接。
func TestStartupPendingClearFailureClosesValidatedConnection(t *testing.T) {
	f := makeStartupPending(t, false, false)
	replacement := f.stagedLocation + ".external"
	writeStartupFixture(t, replacement, "external replacement")
	if err := os.Rename(replacement, f.stagedLocation); err != nil {
		t.Fatal(err)
	}
	marker := readStartupFixture(t, f.marker)
	old := readStartupFixture(t, f.oldDB)
	target := readStartupFixture(t, f.targetDB)
	var validated *sql.DB
	database, err := openStartupDatabase(f.paths, func(path string) (*sql.DB, error) {
		var err error
		validated, err = persistence.OpenExisting(path)
		return validated, err
	})
	if database != nil {
		_ = database.Close()
		t.Fatal("database was adopted despite pending cleanup failure")
	}
	if err == nil || !strings.Contains(err.Error(), "clear pending storage location") {
		t.Fatalf("startup error = %v", err)
	}
	if validated == nil || validated.Ping() == nil {
		t.Fatal("startup did not close the already-validated connection")
	}
	assertStartupPathMissing(t, f.paths.locationFile)
	assertStartupFileUnchanged(t, f.stagedLocation, []byte("external replacement"))
	assertStartupFileUnchanged(t, f.marker, marker)
	assertStartupFileUnchanged(t, f.oldDB, old)
	assertStartupFileUnchanged(t, f.targetDB, target)
}
