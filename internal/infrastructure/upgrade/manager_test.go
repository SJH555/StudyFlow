package upgrade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func testDatabase(t *testing.T, fresh bool) (*persistence.Database, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "study.db")
	var d *persistence.Database
	var err error
	if !fresh {
		d, err = persistence.NewDatabase(path)
	} else {
		db, e := sql.Open("sqlite", path)
		if e != nil {
			t.Fatal(e)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if _, e = db.Exec(`PRAGMA foreign_keys=ON`); e != nil {
			db.Close()
			t.Fatal(e)
		}
		d, err = persistence.AdoptDatabase(db, path)
		if err != nil {
			db.Close()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	return d, path
}
func testManager(t *testing.T, target int64, extra map[int64]string) *Manager {
	t.Helper()
	base, err := scripts.ReadFile("sql/00001_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{"00001_baseline.sql": &fstest.MapFile{Data: base}}
	for v, body := range extra {
		source[fmt.Sprintf("%05d_test.sql", v)] = &fstest.MapFile{Data: []byte("-- +goose Up\n" + body + "\n")}
	}
	m, err := newManager(source, target)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func apply(t *testing.T, d *persistence.Database, m *Manager) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.WithAccess(ctx, func(a *persistence.Access) error {
		s, err := m.Inspect(ctx, a)
		if err != nil {
			return err
		}
		return m.Apply(ctx, a, s)
	})
}
func inspect(t *testing.T, d *persistence.Database, m *Manager) (State, error) {
	t.Helper()
	var s State
	err := d.WithAccess(context.Background(), func(a *persistence.Access) error {
		var err error
		s, err = m.Inspect(context.Background(), a)
		return err
	})
	return s, err
}
func execTest(t *testing.T, d *persistence.Database, query string) {
	t.Helper()
	if err := d.WithWrite(func(db *sql.DB, _ string) error { _, err := db.Exec(query); return err }); err != nil {
		t.Fatal(err)
	}
}
func scalar(t *testing.T, d *persistence.Database, query string) string {
	t.Helper()
	var got string
	if err := d.WithRead(func(db *sql.DB, _ string) error { return db.QueryRow(query).Scan(&got) }); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestBaselineAdoptionPreservesBusinessData(t *testing.T) {
	d, _ := testDatabase(t, false)
	m, _ := New()
	execTest(t, d, `INSERT INTO subjects(id,name,created_at) VALUES(1,'数学','2026-01-01'); INSERT INTO sessions(id,subject_id,start_at,end_at,duration_seconds,source) VALUES(1,1,'a','b',10,'manual'); INSERT INTO stage_goals(id,title,deadline,completed_at,created_at,updated_at) VALUES(1,'目标','2026-12-01','done','a','a')`)
	s, err := inspect(t, d, m)
	if err != nil || !s.Untracked || s.Version != 1 {
		t.Fatalf("%+v %v", s, err)
	}
	if got := scalar(t, d, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
		t.Fatal("inspection modified database")
	}
	if err := apply(t, d, m); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, d, `SELECT deleted FROM sessions WHERE id=1`); got != "0" {
		t.Fatal("repeated old cleanup")
	}
	if got := scalar(t, d, `SELECT count(*) FROM stage_goals WHERE accepted_at IS NULL`); got != "1" {
		t.Fatal("repeated old acceptance conversion")
	}
	if err := apply(t, d, m); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, d, `SELECT group_concat(version_id) FROM goose_db_version`); got != "0,1" {
		t.Fatal(got)
	}
	if got := scalar(t, d, `PRAGMA user_version`); got != "0" {
		t.Fatal("unexpected second version source", got)
	}
}
func TestFreshDatabaseCreatesBaseline(t *testing.T) {
	d, _ := testDatabase(t, true)
	m, _ := New()
	if err := apply(t, d, m); err != nil {
		t.Fatal(err)
	}
	s, err := inspect(t, d, m)
	if err != nil || s.NeedsUpgrade() || s.Version != 1 {
		t.Fatalf("%+v %v", s, err)
	}
}
func TestUntrackedCanJumpDirectlyToTen(t *testing.T) {
	d, _ := testDatabase(t, false)
	extra := map[int64]string{2: `CREATE TABLE upgrade_probe(id INTEGER PRIMARY KEY); INSERT INTO upgrade_probe VALUES(2);`}
	for v := int64(3); v <= 10; v++ {
		extra[v] = fmt.Sprintf(`INSERT INTO upgrade_probe VALUES(%d);`, v)
	}
	m := testManager(t, 10, extra)
	if err := apply(t, d, m); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, d, `SELECT max(version_id) FROM goose_db_version`); got != "10" {
		t.Fatal(got)
	}
	if got := scalar(t, d, `SELECT count(*) FROM upgrade_probe`); got != "9" {
		t.Fatal(got)
	}
	if err := d.WithRead(func(db *sql.DB, _ string) error {
		stats := db.Stats()
		if stats.MaxOpenConnections != 1 || stats.OpenConnections != 1 || stats.InUse != 0 {
			t.Fatalf("unexpected connections %+v", stats)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestPerFileRollbackAndRestart(t *testing.T) {
	d, path := testDatabase(t, false)
	extra := map[int64]string{2: `CREATE TABLE upgrade_probe(id INTEGER PRIMARY KEY,value TEXT); INSERT INTO upgrade_probe VALUES(2,'committed');`, 3: `UPDATE upgrade_probe SET value='wrong'; ALTER TABLE upgrade_probe ADD COLUMN partial TEXT; INSERT INTO missing_table VALUES(1);`}
	m := testManager(t, 3, extra)
	if err := apply(t, d, m); err == nil {
		t.Fatal("expected SQL error")
	}
	if got := scalar(t, d, `SELECT max(version_id) FROM goose_db_version`); got != "2" {
		t.Fatal(got)
	}
	if got := scalar(t, d, `SELECT value FROM upgrade_probe`); got != "committed" {
		t.Fatal(got)
	}
	if got := scalar(t, d, `SELECT count(*) FROM pragma_table_info('upgrade_probe') WHERE name='partial'`); got != "0" {
		t.Fatal("DDL not rolled back")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := persistence.OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := persistence.AdoptDatabase(db, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	extra[3] = `INSERT INTO upgrade_probe VALUES(3,'resumed');`
	if err := apply(t, reopened, testManager(t, 3, extra)); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, reopened, `SELECT count(*) FROM upgrade_probe`); got != "2" {
		t.Fatal(got)
	}
}
func TestVersionRecordFailureRollsBackSQL(t *testing.T) {
	d, _ := testDatabase(t, false)
	extra := map[int64]string{2: `CREATE TABLE upgrade_probe(id INTEGER PRIMARY KEY);`, 3: `INSERT INTO upgrade_probe VALUES(3);`}
	if err := apply(t, d, testManager(t, 2, extra)); err != nil {
		t.Fatal(err)
	}
	execTest(t, d, `CREATE TRIGGER reject_version BEFORE INSERT ON goose_db_version WHEN NEW.version_id=3 BEGIN SELECT RAISE(ABORT,'version failure'); END;`)
	if err := apply(t, d, testManager(t, 3, extra)); err == nil {
		t.Fatal("version insertion should fail")
	}
	if got := scalar(t, d, `SELECT count(*) FROM upgrade_probe`); got != "0" {
		t.Fatal("SQL committed without version")
	}
	if got := scalar(t, d, `SELECT max(version_id) FROM goose_db_version`); got != "2" {
		t.Fatal(got)
	}
}
func TestCommitFailureQuarantinesConnection(t *testing.T) {
	d, path := testDatabase(t, false)
	m := testManager(t, 2, map[int64]string{2: `CREATE TABLE upgrade_parent(id INTEGER PRIMARY KEY); CREATE TABLE upgrade_child(parent_id INTEGER REFERENCES upgrade_parent(id) DEFERRABLE INITIALLY DEFERRED); INSERT INTO upgrade_child VALUES(99);`})
	if err := apply(t, d, m); err == nil {
		t.Fatal("expected deferred constraint commit failure")
	}
	if err := d.WithRead(func(*sql.DB, string) error { t.Error("unhealthy connection was reused"); return nil }); err == nil {
		t.Fatal("database not paused")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := persistence.OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, count int
	if err := db.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("version %d %v", version, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name IN ('upgrade_parent','upgrade_child')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial DDL %d %v", count, err)
	}
}
func TestTargetIsolationAndNewerDatabaseRefusal(t *testing.T) {
	d, _ := testDatabase(t, false)
	extra := map[int64]string{2: `ALTER TABLE settings ADD COLUMN extra TEXT;`}
	if err := apply(t, d, testManager(t, 1, extra)); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, d, `SELECT count(*) FROM pragma_table_info('settings') WHERE name='extra'`); got != "0" {
		t.Fatal("executed above APP target")
	}
	if err := apply(t, d, testManager(t, 2, extra)); err != nil {
		t.Fatal(err)
	}
	m, _ := New()
	_, err := inspect(t, d, m)
	if !errors.Is(err, ErrNewerDatabase) {
		t.Fatal(err)
	}
	if got := scalar(t, d, `SELECT max(version_id) FROM goose_db_version`); got != "2" {
		t.Fatal("newer version was changed")
	}
}
func TestUnknownUntrackedSchemaRejectedWithoutWrites(t *testing.T) {
	for _, change := range []string{`ALTER TABLE subjects ADD COLUMN unknown TEXT`, `DROP TABLE plans`, `CREATE TABLE unknown(id INTEGER)`, `CREATE INDEX unexpected ON sessions(source)`, `CREATE TRIGGER unexpected AFTER INSERT ON settings BEGIN SELECT 1; END;`} {
		t.Run(change, func(t *testing.T) {
			d, _ := testDatabase(t, false)
			execTest(t, d, change)
			m, _ := New()
			_, err := inspect(t, d, m)
			if !errors.Is(err, ErrUnknownSchema) {
				t.Fatal(err)
			}
			if got := scalar(t, d, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
				t.Fatal("wrote ledger before recognition")
			}
		})
	}
}
func TestColumnOrderDoesNotAffectBaseline(t *testing.T) {
	d, _ := testDatabase(t, true)
	base, _ := scripts.ReadFile("sql/00001_baseline.sql")
	text := strings.Replace(string(base), "start_at TEXT NOT NULL, planned_end_at TEXT, end_at", "start_at TEXT NOT NULL, end_at", 1)
	text = strings.Replace(text, "break_used_seconds INTEGER NOT NULL DEFAULT 0)", "break_used_seconds INTEGER NOT NULL DEFAULT 0, planned_end_at TEXT)", 1)
	execTest(t, d, text)
	m, _ := New()
	if err := apply(t, d, m); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidHistoryAndStaleState(t *testing.T) {
	for _, change := range []string{`DELETE FROM goose_db_version WHERE version_id=0`, `UPDATE goose_db_version SET is_applied=0 WHERE version_id=1`, `INSERT INTO goose_db_version(version_id,is_applied) VALUES(1,1)`, `UPDATE goose_db_version SET version_id=3 WHERE version_id=1`} {
		t.Run(change, func(t *testing.T) {
			d, _ := testDatabase(t, false)
			m, _ := New()
			if err := apply(t, d, m); err != nil {
				t.Fatal(err)
			}
			execTest(t, d, change)
			_, err := inspect(t, d, m)
			if !errors.Is(err, ErrInvalidHistory) {
				t.Fatal(err)
			}
		})
	}
	d, _ := testDatabase(t, false)
	m, _ := New()
	s, err := inspect(t, d, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(t, d, m); err != nil {
		t.Fatal(err)
	}
	if err := d.WithAccess(context.Background(), func(a *persistence.Access) error { return m.Apply(context.Background(), a, s) }); err == nil {
		t.Fatal("stale consent accepted")
	}
}
func TestScriptRestrictions(t *testing.T) {
	for _, body := range []string{"-- +goose NO TRANSACTION\nSELECT 1;", "COMMIT;", "SELECT 1; END;", "BEGIN; SELECT 1;", "PRAGMA foreign_keys=OFF;", "-- +goose ENVSUB ON\nSELECT '${SQL}';", "ATTACH 'x' AS other;", "-- +goose StatementBegin\nSELECT 1; COMMIT;\n-- +goose StatementEnd"} {
		t.Run(body, func(t *testing.T) {
			source := fstest.MapFS{"00001_bad.sql": &fstest.MapFile{Data: []byte("-- +goose Up\n" + body)}}
			if _, err := newManager(source, 1); err == nil {
				t.Fatal("unsafe script accepted")
			}
		})
	}
	for _, body := range []string{`SELECT 'COMMIT; END;';`, `SELECT CASE WHEN 1 THEN 2 END;`, "-- COMMIT\nSELECT 1;", `CREATE TRIGGER example AFTER INSERT ON settings BEGIN UPDATE settings SET value='END'; END;`} {
		if err := checkScript(body); err != nil {
			t.Fatalf("safe SQL rejected: %s: %v", body, err)
		}
	}
}
func TestInvalidGooseAnnotationsDoNotRegisterBaseline(t *testing.T) {
	d, _ := testDatabase(t, false)
	source := fstest.MapFS{"00001_bad.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}}
	m, err := newManager(source, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(t, d, m); err == nil {
		t.Fatal("invalid annotation accepted")
	}
	if got := scalar(t, d, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
		t.Fatal("baseline registered before validating scripts")
	}
}

func TestBaselineRejectsDifferentConstraints(t *testing.T) {
	base, _ := scripts.ReadFile("sql/00001_baseline.sql")
	for _, mutation := range []struct{ name, old, new string }{
		{"check", "name TEXT NOT NULL UNIQUE", "name TEXT NOT NULL UNIQUE CHECK(length(name)>0)"},
		{"replace_on_conflict", "name TEXT NOT NULL UNIQUE", "name TEXT NOT NULL UNIQUE ON CONFLICT REPLACE"},
		{"ignore_on_conflict", "name TEXT NOT NULL UNIQUE", "name TEXT NOT NULL UNIQUE ON CONFLICT IGNORE"},
		{"autoincrement", "id INTEGER PRIMARY KEY", "id INTEGER PRIMARY KEY AUTOINCREMENT"},
		{"default", "DEFAULT 'timer'", "DEFAULT 'manual'"},
		{"foreign_key", "subject_id INTEGER NOT NULL REFERENCES subjects(id)", "subject_id INTEGER NOT NULL REFERENCES plans(id)"},
		{"unique", "name TEXT NOT NULL UNIQUE", "name TEXT NOT NULL"},
		{"deferred_foreign_key", "subject_id INTEGER NOT NULL REFERENCES subjects(id)", "subject_id INTEGER NOT NULL REFERENCES subjects(id) DEFERRABLE INITIALLY DEFERRED"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			d, _ := testDatabase(t, true)
			changed := strings.Replace(string(base), mutation.old, mutation.new, 1)
			if changed == string(base) {
				t.Fatal("fixture mutation missed")
			}
			execTest(t, d, changed)
			m, _ := New()
			_, err := inspect(t, d, m)
			if !errors.Is(err, ErrUnknownSchema) {
				t.Fatalf("different constraint accepted: %v", err)
			}
		})
	}
}
func TestIncompleteScriptChainRejected(t *testing.T) {
	for _, source := range []fstest.MapFS{
		{"00001_base.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nSELECT 1;")}},
		{"00001_base.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nSELECT 1;")}, "00001_duplicate.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nSELECT 1;")}},
	} {
		if _, err := newManager(source, 2); err == nil {
			t.Fatal("incomplete/duplicate chain accepted")
		}
	}
}

func TestGooseStatementBlockSupportsTriggers(t *testing.T) {
	d, _ := testDatabase(t, false)
	body := `-- +goose StatementBegin
CREATE TRIGGER upgraded_settings_log AFTER INSERT ON settings BEGIN
 INSERT INTO activity_logs(happened_at,content) VALUES('test','upgraded setting');
END;
-- +goose StatementEnd
INSERT INTO settings(key,value) VALUES('upgrade_probe','yes');`
	if err := apply(t, d, testManager(t, 2, map[int64]string{2: body})); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, d, `SELECT count(*) FROM activity_logs WHERE content='upgraded setting'`); got != "1" {
		t.Fatal(got)
	}
}
