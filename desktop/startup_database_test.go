package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/system"
)

func startupPathsFixture(t *testing.T) startupDatabasePaths {
	t.Helper()
	root := t.TempDir()
	return startupDatabasePaths{
		locationFile: filepath.Join(root, "base", "storage-location.txt"),
		defaultDir:   filepath.Join(root, "base", "data"),
		installDir:   filepath.Join(root, "install"), legacyConfigDir: filepath.Join(root, "old-config"),
	}
}

func writeStartupFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func readStartupFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertStartupFileUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	if after := readStartupFixture(t, path); !bytes.Equal(before, after) {
		t.Fatalf("file changed: %s", path)
	}
}

func assertStartupPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no %s, got %v", path, err)
	}
}

func seedStartupSQLite(t *testing.T, path string, initialize bool, statements ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	open := persistence.Open
	if !initialize {
		open = func(path string) (*sql.DB, error) { return sql.Open("sqlite", path) }
	}
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func openStartupFixture(t *testing.T, paths startupDatabasePaths, want string) *persistence.Database {
	t.Helper()
	db, err := openStartupDatabase(paths, persistence.OpenExisting)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if db.Path() != want {
		t.Fatalf("selected database %q, want %q", db.Path(), want)
	}
	return db
}

func assertStartupValue(t *testing.T, db *persistence.Database, stmt, want string) {
	t.Helper()
	var got string
	err := db.WithRead(func(raw *sql.DB, _ string) error {
		return raw.QueryRow(stmt).Scan(&got)
	})
	if err != nil || got != want {
		t.Fatalf("%s = %q, %v; want %q", stmt, got, err, want)
	}
}

func assertStartupRejected(t *testing.T, paths startupDatabasePaths, want string) {
	t.Helper()
	db, err := openStartupDatabase(paths, persistence.OpenExisting)
	if db != nil {
		_ = db.Close()
		t.Fatal("startup returned a database despite failure")
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("startup error = %v; want %q", err, want)
	}
}

// TestStartupDatabaseFirstLaunchAndDefaultReopen 验证首次启动与默认位置再次打开。
func TestStartupDatabaseFirstLaunchAndDefaultReopen(t *testing.T) {
	p := startupPathsFixture(t)
	path := filepath.Join(p.defaultDir, "study.db")
	first := openStartupFixture(t, p, path)
	assertStartupValue(t, first, `SELECT value FROM settings WHERE key='dailyReviewEnabled'`, "0")
	assertStartupPathMissing(t, p.locationFile)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	db := openStartupFixture(t, p, path)
	assertStartupValue(t, db, `SELECT count(*) FROM sqlite_schema WHERE type='table'`, "9")
	assertStartupValue(t, db, `SELECT count(*) FROM stage_goals`, "0")
}

// TestStartupDatabaseHonorsFormalLocation 验证启动遵循正式的位置文件。
func TestStartupDatabaseHonorsFormalLocation(t *testing.T) {
	for _, kind := range []string{"complete", "missing", "damaged", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			p := startupPathsFixture(t)
			defaultDB := filepath.Join(p.defaultDir, "study.db")
			seedStartupSQLite(t, defaultDB, true)
			defaultBefore := readStartupFixture(t, defaultDB)
			chosen := filepath.Join(filepath.Dir(p.defaultDir), "chosen")
			path := filepath.Join(chosen, "study.db")
			official := "  " + chosen + "\nignored second line\n"
			writeStartupFixture(t, p.locationFile, official)
			switch kind {
			case "complete":
				seedStartupSQLite(t, path, true, `DELETE FROM settings WHERE key='dailyReviewEnabled'`)
				db := openStartupFixture(t, p, path)
				assertStartupValue(t, db, `SELECT value FROM settings WHERE key='dailyReviewEnabled'`, "0")
			case "missing":
				assertStartupRejected(t, p, "缺少 study.db")
				assertStartupPathMissing(t, chosen)
			case "damaged":
				writeStartupFixture(t, path, "damaged database")
				assertStartupRejected(t, p, path)
				assertStartupFileUnchanged(t, path, []byte("damaged database"))
			case "unknown":
				seedStartupSQLite(t, path, false, `CREATE TABLE unrelated (id INTEGER)`)
				before := readStartupFixture(t, path)
				assertStartupRejected(t, p, "unrecognized or incomplete")
				assertStartupFileUnchanged(t, path, before)
			}
			assertStartupFileUnchanged(t, p.locationFile, []byte(official))
			assertStartupFileUnchanged(t, defaultDB, defaultBefore)
		})
	}
}

// 这三个版本复现了在两个历史文件与早期可执行文件中观测到的五张表 DDL，
// 并附带合成的测试数据。
func seedHistoricalStartupDatabase(t *testing.T, path string, version int, transforms ...func(string) string) {
	t.Helper()
	ddl := []string{
		`CREATE TABLE subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL DEFAULT '#7668e8', created_at TEXT NOT NULL)`,
		`CREATE TABLE plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL DEFAULT '1,2,3,4,5,6,0', active INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE sessions (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_at TEXT NOT NULL, end_at TEXT, duration_seconds INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'timer', note TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE activity_logs (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, content TEXT NOT NULL)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO subjects(id,name,created_at) VALUES(1,'legacy subject','2026-11-03')`,
	}
	if version >= 2 {
		ddl = append(ddl, `ALTER TABLE subjects ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`)
	}
	if version == 3 {
		ddl = append(ddl,
			`ALTER TABLE subjects ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE plans ADD COLUMN color TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE sessions ADD COLUMN plan_id INTEGER`,
			`ALTER TABLE sessions ADD COLUMN paused_at TEXT`,
			`ALTER TABLE sessions ADD COLUMN resumed_at TEXT`)
	}
	for _, transform := range transforms {
		for i := range ddl {
			ddl[i] = transform(ddl[i])
		}
	}
	seedStartupSQLite(t, path, false, ddl...)
}

// TestStartupDatabaseUpgradesThreeKnownHistoricalSchemas 验证三个已知历史 schema 的升级。
func TestStartupDatabaseUpgradesThreeKnownHistoricalSchemas(t *testing.T) {
	for i, name := range []string{"study-legacy", "exam-notify-legacy", "study"} {
		t.Run(name, func(t *testing.T) {
			p := startupPathsFixture(t)
			path := filepath.Join(filepath.Dir(p.defaultDir), "chosen", "study.db")
			seedHistoricalStartupDatabase(t, path, i+1)
			writeStartupFixture(t, p.locationFile, filepath.Dir(path)+"\n")
			db := openStartupFixture(t, p, path)
			assertStartupValue(t, db, `SELECT name FROM subjects WHERE id=1`, "legacy subject")
			assertStartupValue(t, db, `SELECT count(*) FROM sqlite_schema WHERE type='table'`, "9")
			assertStartupValue(t, db, `SELECT count(*) FROM stage_goals`, "0")
		})
	}
}

// TestStartupDatabasePromotesLegacyOnlyAfterValidation 验证只有校验通过后才会提升旧位置。
func TestStartupDatabasePromotesLegacyOnlyAfterValidation(t *testing.T) {
	for _, tc := range []struct {
		name, official string
		present, valid bool
	}{
		{name: "no formal location", valid: true},
		{name: "empty formal location", present: true, valid: true},
		{name: "invalid legacy database", present: true, official: "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := startupPathsFixture(t)
			if tc.present {
				writeStartupFixture(t, p.locationFile, tc.official)
			} else if err := os.MkdirAll(filepath.Dir(p.locationFile), 0700); err != nil {
				t.Fatal(err)
			}
			installChoice := filepath.Join(p.installDir, "custom")
			configChoice := filepath.Join(p.legacyConfigDir, "custom")
			writeStartupFixture(t, filepath.Join(p.installDir, "storage-location.txt"), installChoice+"\n")
			writeStartupFixture(t, filepath.Join(p.legacyConfigDir, "storage-location.txt"), configChoice+"\n")
			seedStartupSQLite(t, filepath.Join(configChoice, "study.db"), true)
			path := filepath.Join(installChoice, "study.db")
			if tc.valid {
				seedStartupSQLite(t, path, true)
				openStartupFixture(t, p, path)
				assertStartupFileUnchanged(t, p.locationFile, []byte(installChoice+"\n"))
			} else {
				writeStartupFixture(t, path, "damaged historical selection")
				assertStartupRejected(t, p, path)
				assertStartupFileUnchanged(t, p.locationFile, []byte(tc.official))
				assertStartupFileUnchanged(t, path, []byte("damaged historical selection"))
			}
			assertStartupFileUnchanged(t, filepath.Join(p.installDir, "storage-location.txt"), []byte(installChoice+"\n"))
			assertStartupPathMissing(t, filepath.Join(p.defaultDir, "study.db"))
		})
	}
}

// TestStartupDatabaseCopiesAndValidatesLegacyDefault 验证复制并校验旧默认位置。
func TestStartupDatabaseCopiesAndValidatesLegacyDefault(t *testing.T) {
	for _, good := range []bool{true, false} {
		name := "invalid copy"
		if good {
			name = "valid copy"
		}
		t.Run(name, func(t *testing.T) {
			p := startupPathsFixture(t)
			// 历史安装目录标记仍然选择默认位置。
			writeStartupFixture(t, p.locationFile, p.installDir+"\n")
			old := filepath.Join(p.installDir, "study.db")
			target := filepath.Join(p.defaultDir, "study.db")
			if good {
				seedHistoricalStartupDatabase(t, old, 1)
			} else {
				writeStartupFixture(t, old, "invalid legacy default")
			}
			before := readStartupFixture(t, old)
			if good {
				db := openStartupFixture(t, p, target)
				assertStartupValue(t, db, `SELECT name FROM subjects WHERE id=1`, "legacy subject")
			} else {
				assertStartupRejected(t, p, target)
				assertStartupPathMissing(t, target)
				entries, err := os.ReadDir(p.defaultDir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("failed snapshot left staging files: %v, %v", entries, err)
				}
			}
			assertStartupFileUnchanged(t, old, before)
			assertStartupFileUnchanged(t, p.locationFile, []byte(p.installDir+"\n"))
		})
	}
}

// TestStartupDatabaseLegacySnapshotIncludesUncheckpointedWAL 验证旧库快照包含未 checkpoint 的 WAL 数据。
func TestStartupDatabaseLegacySnapshotIncludesUncheckpointedWAL(t *testing.T) {
	if source := os.Getenv("STUDYFLOW_TEST_LEGACY_WAL"); source != "" {
		db, err := sql.Open("sqlite", source)
		if err != nil {
			t.Fatal(err)
		}
		var mode string
		if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil || mode != "wal" {
			t.Fatalf("journal mode = %q, %v", mode, err)
		}
		if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
			t.Fatal(err)
		}
		before := readStartupFixture(t, source)
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO settings(rowid,key,value) VALUES(41,'only-in-wal','committed')`); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		assertStartupFileUnchanged(t, source, before)
		if len(readStartupFixture(t, source+"-wal")) == 0 {
			t.Fatal("expected a non-empty WAL after commit")
		}
		// 进程退出后留下未 checkpoint 的 WAL，且不保留任何源连接，
		// 以免迁移期间发生 checkpoint。
		os.Exit(0)
	}

	p := startupPathsFixture(t)
	source := filepath.Join(p.installDir, "study.db")
	target := filepath.Join(p.defaultDir, "study.db")
	seedHistoricalStartupDatabase(t, source, 1)
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupDatabaseLegacySnapshotIncludesUncheckpointedWAL$")
	cmd.Env = append(os.Environ(), "STUDYFLOW_TEST_LEGACY_WAL="+source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare uncheckpointed WAL: %v\n%s", err, output)
	}
	before := readStartupFixture(t, source)
	wal := readStartupFixture(t, source+"-wal")
	if len(wal) == 0 {
		t.Fatal("expected committed WAL data to survive source process exit")
	}

	db := openStartupFixture(t, p, target)
	assertStartupValue(t, db, `SELECT value FROM settings WHERE rowid=41 AND key='only-in-wal'`, "committed")
	assertStartupValue(t, db, `SELECT count(*) FROM sqlite_schema WHERE type='table'`, "9")
	assertStartupValue(t, db, `SELECT count(*) FROM stage_goals`, "0")
	assertStartupFileUnchanged(t, source, before)
	assertStartupFileUnchanged(t, source+"-wal", wal)
	assertStartupPathMissing(t, p.locationFile)
}

// TestStartupDatabaseLegacySnapshotStillRejectsUnknownSchema 验证旧库快照仍拒绝未知 schema。
func TestStartupDatabaseLegacySnapshotStillRejectsUnknownSchema(t *testing.T) {
	p := startupPathsFixture(t)
	source := filepath.Join(p.installDir, "study.db")
	target := filepath.Join(p.defaultDir, "study.db")
	seedStartupSQLite(t, source, false, `CREATE TABLE unrelated (id INTEGER)`)
	before := readStartupFixture(t, source)

	assertStartupRejected(t, p, "unrecognized or incomplete StudyFlow schema")
	assertStartupFileUnchanged(t, source, before)
	// 可能存在合法的 SQLite 快照，但启动阶段不得去初始化它。
	assertStartupRejected(t, p, "unrecognized or incomplete StudyFlow schema")
	db, err := sql.Open("sqlite", target)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tables string
	if err := db.QueryRow(`SELECT group_concat(name) FROM sqlite_schema WHERE type='table'`).Scan(&tables); err != nil || tables != "unrelated" {
		t.Fatalf("unexpected snapshot tables: %q, %v", tables, err)
	}
}

// TestStartupDatabasePositionReadFailureStopsSelection 验证位置读取失败会中止数据库选择。
func TestStartupDatabasePositionReadFailureStopsSelection(t *testing.T) {
	p := startupPathsFixture(t)
	if err := os.MkdirAll(p.locationFile, 0700); err != nil {
		t.Fatal(err)
	}
	assertStartupRejected(t, p, "storage location is not a regular file")
	assertStartupPathMissing(t, filepath.Join(p.defaultDir, "study.db"))
}

// TestStartupDatabaseFailureBlocksBusinessAndBackgroundWork 验证启动失败只允许错误界面，不启动业务或后台工作。
func TestStartupDatabaseFailureBlocksBusinessAndBackgroundWork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("isolated HOME and APPIMAGE exercise the Linux main entry point")
	}
	if os.Getenv("STUDYFLOW_B1_STARTUP_TEST") == "missing" {
		err := runWithStartupPresenter(func(store *App, _ system.InstanceGuard, _, _, _ string, startup *startupUpgrade) error {
			defer store.Close()
			if startup.isReady() {
				t.Fatal("failed database admitted business startup")
			}
			service := &APIService{app: store, startup: startup}
			if _, _, err := service.beginWorkContext(); err == nil {
				t.Fatal("business binding admitted work")
			}
			return errors.New(startup.snapshot().Detail)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(97)
	}
	p := startupPathsFixture(t)
	home := filepath.Join(filepath.Dir(p.defaultDir), "home")
	base := filepath.Join(home, ".StudyFlow")
	location := filepath.Join(base, "storage-location.txt")
	selected := filepath.Join(base, "missing-selection")
	writeStartupFixture(t, location, selected+"\n")
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupDatabaseFailureBlocksBusinessAndBackgroundWork$")
	cmd.Env = append(os.Environ(), "STUDYFLOW_B1_STARTUP_TEST=missing", "HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"), "XDG_DATA_HOME="+filepath.Join(home, "share"),
		"APPIMAGE="+filepath.Join(p.installDir, "StudyFlow.AppImage"))
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(output), "缺少 study.db") {
		t.Fatalf("main did not stop at database selection: %v\n%s", err, output)
	}
	assertStartupPathMissing(t, selected)
	// 桌面入口只有在数据库交接之后才会到达铃声设置与后台工作阶段。
	// 默认数据目录此时仍不存在。
	assertStartupPathMissing(t, filepath.Join(base, "data"))
	assertStartupFileUnchanged(t, location, []byte(selected+"\n"))
}
