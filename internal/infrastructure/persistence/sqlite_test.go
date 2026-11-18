package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenInitializesAndOpenExistingLeavesDataUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study #100%.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var enabled string
	if err := db.QueryRow("SELECT value FROM settings WHERE key='dailyReviewEnabled'").Scan(&enabled); err != nil || enabled != "0" {
		t.Fatalf("startup default = %q, %v", enabled, err)
	}
	if _, err := db.Exec("DELETE FROM settings WHERE key='dailyReviewEnabled'"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO subjects(name,color,created_at) VALUES('legacy','#7668e8','2026-11-05')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var color string
	if err := db.QueryRow("SELECT color FROM subjects WHERE name='legacy'").Scan(&color); err != nil || color != "#7668e8" {
		t.Fatalf("existing color = %q, %v", color, err)
	}
	if err := db.QueryRow("SELECT value FROM settings WHERE key='dailyReviewEnabled'").Scan(&enabled); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("existing database gained startup default: %v", err)
	}
}

func TestOpenExistingRejectsMissingCorruptAndUninitializedFiles(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.db")
	if db, err := OpenExisting(missing); err == nil || db != nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatalf("opened missing database: %v", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file should not have been created: %v", err)
	}
	for name, data := range map[string][]byte{"corrupt.db": []byte("not a SQLite database"), "empty.db": nil} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if db, err := OpenExisting(path); err == nil || db != nil {
			if db != nil {
				_ = db.Close()
			}
			t.Fatalf("opened %s: %v", name, err)
		}
	}
}

func TestOpenExistingRejectsMissingRequiredColumnsWithoutMigrating(t *testing.T) {
	for _, tc := range []struct{ table, column string }{
		{"subjects", "sort_order"},
		{"plans", "note"},
		{"sessions", "planned_end_at"},
		{"activity_logs", "content"},
		{"settings", "value"},
		{"daily_reviews", "submitted_at"},
		{"daily_review_items", "reason"},
		{"daily_plan_targets", "targets_json"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			_, alterErr := db.Exec("ALTER TABLE " + tc.table + " RENAME COLUMN " + tc.column + " TO missing_column_fixture")
			if err := errors.Join(alterErr, db.Close()); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := OpenExisting(path)
			if candidate != nil {
				_ = candidate.Close()
				t.Fatal("opened a database with a missing required column")
			}
			if err == nil || !strings.Contains(err.Error(), "t."+tc.column) {
				t.Fatalf("expected missing column %s.%s, got %v", tc.table, tc.column, err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("validation changed the database file: %v", err)
			}
		})
	}
}

func TestOpenExistingValidatesPreparedWALSnapshotWithoutWritingData(t *testing.T) {
	db, source, location := directoryFixture(t)
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("source journal mode = %q, %v", mode, err)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	mainBefore, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key, value) VALUES ('a21-wal-only', 'committed')`); err != nil {
		t.Fatal(err)
	}
	mainAfter, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(mainBefore, mainAfter) {
		t.Fatalf("fixture write was not isolated to the WAL: %v", err)
	}
	if info, err := os.Stat(source + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("missing source WAL data: %v, %v", info, err)
	}
	prepared, err := prepareDirectory(context.Background(), db, source, filepath.Join(filepath.Dir(location), "target"), location)
	if err != nil {
		t.Fatal(err)
	}
	path := prepared.databasePath
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		assertDirectoryMissing(t, path+suffix)
	}
	candidate, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = candidate.Close() })
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		info, err := os.Stat(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("candidate validation created %s (%d bytes)", suffix, info.Size())
		if suffix == "-journal" || (suffix == "-wal" && info.Size() != 0) {
			t.Fatalf("candidate validation left persistent journal data in %s", suffix)
		}
	}
	var value string
	if err := candidate.QueryRow(`SELECT value FROM settings WHERE key='a21-wal-only'`).Scan(&value); err != nil || value != "committed" {
		t.Fatalf("snapshot WAL row = %q, %v", value, err)
	}
	whileOpen, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, whileOpen) {
		t.Fatalf("candidate file changed during validation: %v", err)
	}
	if err := candidate.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		assertDirectoryMissing(t, path+suffix)
	}
	afterClose, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, afterClose) {
		t.Fatalf("candidate file changed after closing: %v", err)
	}
	if err := prepared.cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteForeignKeysOnReplacementConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	for _, open := range []func(string) (*sql.DB, error){Open, OpenExisting} {
		db, err := open(path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxIdleConns(0)
		for range 2 {
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var enabled int
			if err := conn.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
				t.Fatalf("foreign_keys = %d, %v", enabled, err)
			}
			_, err = conn.ExecContext(context.Background(), "INSERT INTO plans(subject_id,start_time,duration_minutes) VALUES(999,'09:00',60)")
			if err == nil {
				t.Fatal("foreign key violation was accepted")
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLiteInitializationFailureClosesHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	var failed *sql.DB
	db, err := openSQLite(path, true, func(candidate *sql.DB) error {
		failed = candidate
		if _, err := candidate.Exec("CREATE TABLE settings (unrelated TEXT)"); err != nil {
			t.Fatal(err)
		}
		return initializeSchema(candidate)
	})
	if err == nil || db != nil || failed == nil {
		t.Fatalf("initialization should fail: %v", err)
	}
	if err := failed.Ping(); err == nil {
		t.Fatal("failed database handle is still open")
	}
	if stats := failed.Stats(); stats.OpenConnections != 0 {
		t.Fatalf("failed database still holds %d connections", stats.OpenConnections)
	}
}

func TestCloseAfterErrorPreservesBothErrors(t *testing.T) {
	primary := errors.New("initialization failed")
	closeErr := errors.New("close failed")
	got := closeAfterError(primary, func() error { return closeErr })
	if !errors.Is(got, primary) || !errors.Is(got, closeErr) {
		t.Fatalf("expected both errors, got %v", got)
	}
	var closeFailure *sqliteCloseError
	if !errors.As(got, &closeFailure) || !errors.Is(closeFailure, closeErr) {
		t.Fatalf("expected an identifiable close error, got %v", got)
	}
	if got.Error() != "initialization failed\nclose SQLite database: close failed" {
		t.Fatalf("unexpected error text: %q", got)
	}
	if got := closeAfterError(primary, func() error { return nil }); got != primary || errors.As(got, &closeFailure) {
		t.Fatalf("successful close should return only the primary error, got %v", got)
	}
}

func TestOpenForStartupUpgradesVerifiedFiveTableSchemas(t *testing.T) {
	for _, profile := range []string{"study-legacy", "exam-notify-legacy", "study"} {
		t.Run(profile, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unrelated-file-name.db")
			statements := historicalStudyFlowDDL(profile)
			statements = append(statements,
				`INSERT INTO subjects(id,name,created_at) VALUES(7,'historical subject','2026-11-03')`,
				`INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(8,7,'09:00',40)`,
				`INSERT INTO sessions(id,subject_id,start_at,duration_seconds,note)
				  VALUES(9,7,'2026-11-03T09:00:00',600,'session note')`,
				`INSERT INTO activity_logs(id,happened_at,content) VALUES(10,'2026-11-03T09:10:00','activity')`,
				`INSERT INTO settings(key,value) VALUES('dailyReviewEnabled','1')`)
			execSQLiteFixture(t, path, statements...)
			assertStartupRejectsWithoutWrites(t, path, OpenExisting, "missing StudyFlow tables")

			db, err := OpenForStartup(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = OpenExisting(path)
			if err != nil {
				t.Fatalf("upgraded database failed the strict eight-table validation: %v", err)
			}
			defer db.Close()

			for _, tc := range []struct{ query, want string }{
				{`SELECT name FROM subjects WHERE id=7`, "historical subject"},
				{`SELECT color FROM subjects WHERE id=7`, "#007AFF"},
				{`SELECT duration_minutes FROM plans WHERE id=8 AND subject_id=7`, "40"},
				{`SELECT note FROM plans WHERE id=8`, ""},
				{`SELECT note FROM sessions WHERE id=9 AND subject_id=7 AND duration_seconds=600`, "session note"},
				{`SELECT content FROM activity_logs WHERE id=10`, "activity"},
				{`SELECT value FROM settings WHERE key='dailyReviewEnabled'`, "1"},
			} {
				var got string
				if err := db.QueryRow(tc.query).Scan(&got); err != nil || got != tc.want {
					t.Errorf("%s: got %q, %v; want %q", tc.query, got, err, tc.want)
				}
			}
			var enabled int
			if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil || enabled != 1 {
				t.Fatalf("foreign keys = %d, %v", enabled, err)
			}
		})
	}
}

// 基础 DDL 来自 study-legacy.db；仅含 archived 列的变体来自 exam_notify-legacy，其余新增来自 study.db。此处重排 settings 列并以不同格式书写 DDL，以验证匹配依赖 SQLite 元数据而非文本或列顺序。
func historicalStudyFlowDDL(profile string) []string {
	ddl := []string{
		`CREATE TABLE subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL DEFAULT '#7668e8', created_at TEXT NOT NULL)`,
		`CREATE TABLE plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL DEFAULT '1,2,3,4,5,6,0', active INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE sessions (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_at TEXT NOT NULL, end_at TEXT, duration_seconds INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'timer', note TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE activity_logs (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, content TEXT NOT NULL)`,
		`create table "settings" (
			value text not null,
			key text primary key
		)`,
	}
	if profile == "exam-notify-legacy" || profile == "study" {
		ddl = append(ddl, `ALTER TABLE subjects ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`)
	}
	if profile == "study" {
		ddl = append(ddl,
			`ALTER TABLE subjects ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE plans ADD COLUMN color TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE sessions ADD COLUMN plan_id INTEGER`,
			`ALTER TABLE sessions ADD COLUMN paused_at TEXT`,
			`ALTER TABLE sessions ADD COLUMN resumed_at TEXT`)
	}
	return ddl
}

func TestOpenForStartupAcceptsCompleteSchemaAndKeepsStartupWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("initialize first database: %v", err)
	}
	_, writeErr := db.Exec(`DELETE FROM settings WHERE key IN ('dailyReviewStartDate','dailyReviewEnabled');
		INSERT INTO subjects(name,color,created_at) VALUES('subject','#7668e8','2026-11-03')`)
	if err := errors.Join(writeErr, db.Close()); err != nil {
		t.Fatal(err)
	}
	db, err = OpenForStartup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var defaults int
	if err := db.QueryRow(`SELECT count(*) FROM settings
		WHERE (key='dailyReviewStartDate' AND value<>'') OR (key='dailyReviewEnabled' AND value='0')`).
		Scan(&defaults); err != nil || defaults != 2 {
		t.Fatalf("restored startup defaults = %d, %v", defaults, err)
	}
	var color string
	if err := db.QueryRow(`SELECT color FROM subjects WHERE name='subject'`).Scan(&color); err != nil || color != "#007AFF" {
		t.Fatalf("startup color = %q, %v", color, err)
	}
}

func TestOpenForStartupRejectsIncompleteModernSchemasWithoutWrites(t *testing.T) {
	for _, change := range []string{
		`DROP TABLE subjects`, `DROP TABLE plans`, `DROP TABLE sessions`, `DROP TABLE activity_logs`,
		`DROP TABLE settings`, `DROP TABLE daily_reviews`, `DROP TABLE daily_review_items`, `DROP TABLE daily_plan_targets`,
		`ALTER TABLE activity_logs DROP COLUMN content`,
		`ALTER TABLE plans DROP COLUMN note`,
		`DROP TABLE daily_plan_targets; DROP TABLE daily_review_items; DROP TABLE daily_reviews`,
	} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			execSQLiteFixture(t, path, `DELETE FROM settings`, change)
			assertStartupRejectsWithoutWrites(t, path, OpenForStartup, "StudyFlow")
		})
	}
}

func TestOpenForStartupRejectsUnsupportedFiveTableSchemasWithoutWrites(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tableDDL int
		from, to string
		extra    string
	}{
		{name: "partial review schema", extra: `CREATE TABLE daily_plan_targets (day TEXT PRIMARY KEY, targets_json TEXT NOT NULL)`},
		{name: "mixed historical columns", extra: `ALTER TABLE plans ADD COLUMN color TEXT NOT NULL DEFAULT ''`},
		{name: "missing historical column", extra: `ALTER TABLE activity_logs DROP COLUMN content`},
		{name: "unknown table", extra: `CREATE TABLE another (id INTEGER)`},
		{name: "unexpected trigger", extra: `CREATE TRIGGER another AFTER INSERT ON settings BEGIN SELECT 1; END`},
		{name: "wrong primary key", tableDDL: 3, from: "id INTEGER PRIMARY KEY", to: "id INTEGER"},
		{name: "nonunique subject name", from: "name TEXT NOT NULL UNIQUE", to: "name TEXT NOT NULL"},
		{name: "wrong unique key", from: "created_at TEXT NOT NULL)", to: "created_at TEXT NOT NULL, UNIQUE (name, created_at))"},
		{name: "wrong foreign key", tableDDL: 1, from: "REFERENCES subjects(id)", to: "REFERENCES subjects(id) ON DELETE CASCADE"},
		{name: "wrong nullability", tableDDL: 2, from: "end_at TEXT", to: "end_at TEXT NOT NULL"},
		{name: "wrong default", tableDDL: 2, from: "DEFAULT 'timer'", to: "DEFAULT 'other'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			ddl := historicalStudyFlowDDL("study-legacy")
			if tc.from != "" {
				ddl[tc.tableDDL] = strings.Replace(ddl[tc.tableDDL], tc.from, tc.to, 1)
			}
			if tc.extra != "" {
				ddl = append(ddl, tc.extra)
			}
			execSQLiteFixture(t, path, ddl...)
			assertStartupRejectsWithoutWrites(t, path, OpenForStartup, "unrecognized or incomplete")
		})
	}
}

func TestOpenForStartupRejectsMissingEmptyUnrelatedAndCorruptFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	if db, err := OpenForStartup(missing); err == nil || db != nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatalf("opened missing database: %v", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing database was created: %v", err)
	}

	for _, tc := range []struct {
		name       string
		bytes      []byte
		statements []string
		want       string
	}{
		{name: "two table migration fixture", statements: []string{
			`CREATE TABLE subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL, created_at TEXT NOT NULL, archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0)`,
			`CREATE TABLE plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1, color TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0, break_minutes INTEGER NOT NULL DEFAULT 0)`,
			`INSERT INTO subjects(id,name,color,created_at) VALUES(1,'subject','#007AFF','2026-11-01')`,
			`INSERT INTO plans(id,subject_id,start_time,duration_minutes,weekdays) VALUES(1,1,'09:00',60,'1,2,3,4,5')`,
		}, want: "unrecognized or incomplete"},
		{name: "zero bytes", bytes: []byte{}, want: "unrecognized or incomplete"},
		{name: "SQLite with no user tables", statements: []string{`VACUUM`}, want: "unrecognized or incomplete"},
		{name: "unrelated SQLite", statements: []string{`CREATE TABLE unrelated (key TEXT PRIMARY KEY)`}, want: "unrecognized or incomplete"},
		{name: "non-SQLite", bytes: []byte("not a SQLite database")},
		{name: "quick check fails", statements: []string{
			`CREATE TABLE broken (value INTEGER CHECK(value > 0))`,
			`PRAGMA ignore_check_constraints=ON`, `INSERT INTO broken VALUES(-1)`,
		}, want: "SQLite integrity check failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			if tc.bytes != nil {
				if err := os.WriteFile(path, tc.bytes, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				execSQLiteFixture(t, path, tc.statements...)
			}
			assertStartupRejectsWithoutWrites(t, path, OpenForStartup, tc.want)
		})
	}
}

func TestOpenForStartupRollsBackEntireMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	execSQLiteFixture(t, path,
		`ALTER TABLE stage_goals DROP COLUMN accepted_at`,
		`DELETE FROM settings WHERE key='dailyReviewEnabled'`,
		`INSERT INTO subjects(name,color,created_at) VALUES('subject','#7668e8','2026-11-03')`,
		`CREATE TRIGGER fail_color BEFORE UPDATE OF color ON subjects
		  BEGIN SELECT RAISE(ABORT, 'color migration failed in fixture'); END`)

	db, err = OpenForStartup(path)
	if db != nil {
		_ = db.Close()
		t.Fatal("startup returned a usable handle after a migration error")
	}
	if err == nil || !strings.Contains(err.Error(), "StudyFlow startup migration failed") ||
		!strings.Contains(err.Error(), "color migration failed in fixture") {
		t.Fatalf("migration error = %v", err)
	}
	db, err = OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM settings WHERE key='dailyReviewEnabled'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("earlier startup write was not rolled back: count=%d, err=%v", count, err)
	}
	var columns int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('stage_goals') WHERE name='accepted_at'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("DDL was not rolled back: count=%d, err=%v", columns, err)
	}
}

func execSQLiteFixture(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func assertStartupRejectsWithoutWrites(t *testing.T, path string, open func(string) (*sql.DB, error), wantError string) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := open(path)
	if db != nil {
		_ = db.Close()
		t.Fatalf("unexpected database handle: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("open error = %v, want %q", err, wantError)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected database file changed: %v", err)
	}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected database left %s: %v", suffix, err)
		}
	}
}
