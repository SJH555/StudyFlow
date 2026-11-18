package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

func prepareUpgradeFixture(t *testing.T, seed bool) (startupDatabasePaths, *App, *startupUpgrade) {
	t.Helper()
	p := startupPathsFixture(t)
	if seed {
		seedStartupSQLite(t, filepath.Join(p.defaultDir, "study.db"), true, `INSERT INTO subjects(id,name,created_at) VALUES(91,'保留项目','2026-01-01'); INSERT INTO sessions(id,subject_id,start_at,end_at,duration_seconds,source) VALUES(92,91,'a','b',60,'manual'); INSERT INTO stage_goals(id,title,deadline,completed_at,created_at,updated_at) VALUES(93,'保留目标','2026-12-31','done','a','a')`)
	}
	app, c := prepareStartupUpgrade(p)
	t.Cleanup(func() {
		app.tasks.stopAndWait()
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})
	return p, app, c
}
func upgradeScalar(t *testing.T, d *persistence.Database, query string) string {
	t.Helper()
	var value string
	if err := d.WithRead(func(db *sql.DB, _ string) error { return db.QueryRow(query).Scan(&value) }); err != nil {
		t.Fatal(err)
	}
	return value
}
func rawUpgradeScalar(t *testing.T, path, query string) string {
	t.Helper()
	db, err := persistence.OpenUnprepared(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow(query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestUpgradeStartupConsentBackupAndRepeat(t *testing.T) {
	p, app, c := prepareUpgradeFixture(t, true)
	before := readStartupFixture(t, c.snapshot().DatabasePath)
	if state := c.snapshot(); state.Phase != "confirmation" || state.Kind != "baseline" {
		t.Fatalf("%+v", state)
	}
	assertStartupFileUnchanged(t, c.snapshot().DatabasePath, before)
	api := &APIService{app: app, startup: c}
	if got := api.Call("subjects", "GET", ""); !strings.Contains(got, "尚未准备") {
		t.Fatal("business API accessed unapproved database", got)
	}
	if err := api.SetDataDirectory(filepath.Join(t.TempDir(), "target")); err == nil {
		t.Fatal("directory change admitted before upgrade")
	}
	if got := upgradeScalar(t, app.database, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
		t.Fatal("inspection registered baseline")
	}
	activated := 0
	c.activate = func() error { activated++; return nil }
	state := (&StartupService{control: c}).Confirm()
	if state.Phase != "ready" || activated != 1 {
		t.Fatalf("%+v activate=%d", state, activated)
	}
	if state.BackupPath == "" {
		t.Fatal("missing verified backup")
	}
	if got := rawUpgradeScalar(t, state.BackupPath, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
		t.Fatal("backup was taken after upgrade")
	}
	if got := rawUpgradeScalar(t, state.BackupPath, `SELECT name FROM subjects WHERE id=91`); got != "保留项目" {
		t.Fatal(got)
	}
	if got := upgradeScalar(t, app.database, `SELECT deleted FROM sessions WHERE id=92`); got != "0" {
		t.Fatal("historical cleanup reran")
	}
	if got := upgradeScalar(t, app.database, `SELECT count(*) FROM stage_goals WHERE id=93 AND accepted_at IS NULL`); got != "1" {
		t.Fatal("acceptance conversion reran")
	}
	if got := api.Call("subjects", "GET", ""); strings.Contains(got, `"error"`) {
		t.Fatal(got)
	}
	(&StartupService{control: c}).Confirm()
	if activated != 1 {
		t.Fatal("duplicate confirmation repeated initialization")
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, next := prepareStartupUpgrade(p)
	defer reopened.Close()
	if !next.isReady() {
		t.Fatal(next.snapshot())
	}
	entries, err := os.ReadDir(filepath.Join(p.defaultDir, "backups"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("repeated startup made new backups: %v %v", entries, err)
	}
}
func TestUpgradeFreshStartupInitializesOnce(t *testing.T) {
	p, app, c := prepareUpgradeFixture(t, false)
	if state := c.snapshot(); state.Phase != "ready" || state.Current != 1 {
		t.Fatal(state)
	}
	if got := upgradeScalar(t, app.database, `SELECT value FROM settings WHERE key='dailyReviewEnabled'`); got != "0" {
		t.Fatal(got)
	}
	if got := upgradeScalar(t, app.database, `SELECT value FROM settings WHERE key='dailyReviewStartDate'`); got != time.Now().Format("2006-01-02") {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(p.defaultDir, "backups")); !os.IsNotExist(err) {
		t.Fatal("new empty database required a backup", err)
	}
}
func TestUpgradeCancelAndFailuresPreserveUnregisteredDatabase(t *testing.T) {
	for _, failure := range []string{"cancel", "backup", "panic", "shutdown"} {
		t.Run(failure, func(t *testing.T) {
			_, app, c := prepareUpgradeFixture(t, true)
			path := c.snapshot().DatabasePath
			before := readStartupFixture(t, path)
			if failure == "cancel" {
				if err := app.Close(); err != nil {
					t.Fatal(err)
				}
				assertStartupFileUnchanged(t, path, before)
				return
			}
			marker := errors.New("injected backup failure")
			c.backup = func(ctx context.Context, _ *persistence.Access) (string, error) {
				if failure == "panic" {
					panic("backup panic")
				}
				if failure == "shutdown" {
					<-ctx.Done()
					return "", ctx.Err()
				}
				return "", marker
			}
			var state startupStatus
			if failure == "shutdown" {
				done := make(chan startupStatus, 1)
				go func() { done <- (&StartupService{control: c}).Confirm() }()
				deadline := time.After(3 * time.Second)
				for c.snapshot().Phase != "upgrading" {
					select {
					case <-deadline:
						t.Fatal("confirm did not start")
					default:
						time.Sleep(time.Millisecond)
					}
				}
				app.tasks.stopAndWait()
				select {
				case state = <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("shutdown did not drain upgrade")
				}
			} else {
				state = (&StartupService{control: c}).Confirm()
			}
			if state.Phase != "blocked" {
				t.Fatal(state)
			}
			if c.isReady() {
				t.Fatal("failure admitted business")
			}
			assertStartupFileUnchanged(t, path, before)
		})
	}
}
func TestUpgradeActualBackupFailureDoesNotRegister(t *testing.T) {
	p, _, c := prepareUpgradeFixture(t, true)
	path := c.snapshot().DatabasePath
	before := readStartupFixture(t, path)
	if err := os.WriteFile(filepath.Join(p.defaultDir, "backups"), []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	state := (&StartupService{control: c}).Confirm()
	if state.Phase != "blocked" || !strings.Contains(state.Detail, "备份失败") {
		t.Fatal(state)
	}
	assertStartupFileUnchanged(t, path, before)
}
func TestUpgradeConcurrentConfirmationRunsOnce(t *testing.T) {
	_, _, c := prepareUpgradeFixture(t, true)
	original := c.backup
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c.backup = func(ctx context.Context, a *persistence.Access) (string, error) {
		calls.Add(1)
		close(entered)
		<-release
		return original(ctx, a)
	}
	done := make(chan startupStatus, 1)
	go func() { done <- (&StartupService{control: c}).Confirm() }()
	<-entered
	if state := (&StartupService{control: c}).Confirm(); state.Phase != "upgrading" {
		t.Fatal(state)
	}
	close(release)
	select {
	case state := <-done:
		if state.Phase != "ready" || calls.Load() != 1 {
			t.Fatal(state, calls.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upgrade deadlocked")
	}
}
func TestUpgradeBlocksNewerUnknownAndMissingWithoutWrites(t *testing.T) {
	for _, kind := range []string{"newer", "unknown", "missing"} {
		t.Run(kind, func(t *testing.T) {
			p := startupPathsFixture(t)
			path := filepath.Join(p.defaultDir, "study.db")
			if kind == "newer" {
				seedStartupSQLite(t, path, true, `CREATE TABLE goose_db_version(id INTEGER PRIMARY KEY,version_id INTEGER,is_applied INTEGER,tstamp TIMESTAMP);INSERT INTO goose_db_version VALUES(1,0,1,NULL),(2,1,1,NULL),(3,2,1,NULL);`)
			}
			if kind == "unknown" {
				seedStartupSQLite(t, path, false, `CREATE TABLE unrelated(id INTEGER)`)
			}
			if kind == "missing" {
				writeStartupFixture(t, p.locationFile, p.defaultDir+"\n")
			}
			var before []byte
			if kind != "missing" {
				before = readStartupFixture(t, path)
			}
			app, c := prepareStartupUpgrade(p)
			defer app.Close()
			if state := c.snapshot(); state.Phase != "blocked" || (kind == "newer" && (state.Kind != "newer" || state.Current != 2)) {
				t.Fatal(state)
			}
			api := &APIService{app: app, startup: c}
			if _, _, err := api.beginWorkContext(); err == nil {
				t.Fatal("blocked binding admitted work")
			}
			if kind != "missing" {
				assertStartupFileUnchanged(t, path, before)
			} else {
				assertStartupPathMissing(t, path)
			}
		})
	}
}
func TestUpgradeKnownHistoricalProfilesAreBackedUpBeforeConversion(t *testing.T) {
	for profile := 1; profile <= 3; profile++ {
		t.Run(string(rune('0'+profile)), func(t *testing.T) {
			p := startupPathsFixture(t)
			path := filepath.Join(p.defaultDir, "study.db")
			seedHistoricalStartupDatabase(t, path, profile)
			app, c := prepareStartupUpgrade(p)
			defer app.Close()
			if state := c.snapshot(); state.Phase != "confirmation" || state.Kind != "historical" {
				t.Fatal(state)
			}
			state := (&StartupService{control: c}).Confirm()
			if state.Phase != "ready" {
				t.Fatal(state)
			}
			if got := rawUpgradeScalar(t, state.BackupPath, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name NOT GLOB 'sqlite_*'`); got != "5" {
				t.Fatal("backup was converted", got)
			}
			if got := upgradeScalar(t, app.database, `SELECT name FROM subjects WHERE id=1`); got != "legacy subject" {
				t.Fatal(got)
			}
		})
	}
	for _, profile := range []string{"missing-goals", "missing-acceptance"} {
		t.Run(profile, func(t *testing.T) {
			p := startupPathsFixture(t)
			path := filepath.Join(p.defaultDir, "study.db")
			change := `DROP TABLE stage_goals`
			if profile == "missing-acceptance" {
				change = `ALTER TABLE stage_goals DROP COLUMN accepted_at;INSERT INTO stage_goals(id,title,deadline,completed_at,created_at,updated_at) VALUES(1,'旧目标','a','done','a','a')`
			}
			seedStartupSQLite(t, path, true, change)
			app, c := prepareStartupUpgrade(p)
			defer app.Close()
			state := (&StartupService{control: c}).Confirm()
			if state.Phase != "ready" {
				t.Fatal(state)
			}
			if got := upgradeScalar(t, app.database, `SELECT count(*) FROM pragma_table_info('stage_goals') WHERE name='accepted_at'`); got != "1" {
				t.Fatal(got)
			}
			if profile == "missing-acceptance" {
				if got := upgradeScalar(t, app.database, `SELECT accepted_at FROM stage_goals WHERE id=1`); got != "done" {
					t.Fatal(got)
				}
			}
		})
	}
}
func TestUpgradeBackupSurvivesRetentionAndDirectorySwitch(t *testing.T) {
	p, app, c := prepareUpgradeFixture(t, true)
	state := (&StartupService{control: c}).Confirm()
	if state.Phase != "ready" {
		t.Fatal(state)
	}
	files, err := persistence.ListBackups(app.database.Path(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.RemoveExpiredBackups(app.database.Path(), files, time.Now().AddDate(20, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state.BackupPath); err != nil {
		t.Fatal("upgrade backup expired", err)
	}
	target := filepath.Join(t.TempDir(), "moved")
	result, err := app.database.SwitchDirectory(context.Background(), target, p.locationFile, p.installDir)
	if err != nil || !result.Committed {
		t.Fatal(result, err)
	}
	backup := filepath.Join(target, "backups", filepath.Base(state.BackupPath))
	if got := rawUpgradeScalar(t, backup, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
		t.Fatal(got)
	}
	if got := upgradeScalar(t, app.database, `SELECT max(version_id) FROM goose_db_version`); got != "1" {
		t.Fatal(got)
	}
}
func TestUpgradeDirectorySwitchRejectsUnexpectedSourceVersion(t *testing.T) {
	p, app, c := prepareUpgradeFixture(t, false)
	if err := app.database.WithWrite(func(db *sql.DB, _ string) error {
		_, err := db.Exec(`INSERT INTO goose_db_version(version_id,is_applied) VALUES(2,1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	result, err := app.database.SwitchDirectory(context.Background(), target, p.locationFile, p.installDir)
	if err == nil || result.Committed {
		t.Fatal("unexpected version switched", result, err)
	}
	assertStartupPathMissing(t, target)
	if !c.isReady() {
		t.Fatal("test changed startup gate rather than testing handoff validation")
	}
}

func TestUpgradeExistingEmptyFileIsNotInitialized(t *testing.T) {
	p := startupPathsFixture(t)
	path := filepath.Join(p.defaultDir, "study.db")
	writeStartupFixture(t, path, "")
	app, c := prepareStartupUpgrade(p)
	defer app.Close()
	if state := c.snapshot(); state.Phase != "blocked" || !strings.Contains(state.Detail, "数据库为空") {
		t.Fatal(state)
	}
	// Opening may create a SQLite header, but never business tables or a version.
	if got := rawUpgradeScalar(t, path, `SELECT count(*) FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*'`); got != "0" {
		t.Fatal("existing empty file was initialized", got)
	}
}
func TestUpgradeRecoveryRejectsNewerBeforeRetiringOldLocation(t *testing.T) {
	f := makeStartupPending(t, true, false)
	seedStartupSQLite(t, f.targetDB, true, `CREATE TABLE goose_db_version(id INTEGER PRIMARY KEY,version_id INTEGER,is_applied INTEGER,tstamp TIMESTAMP);INSERT INTO goose_db_version VALUES(1,0,1,NULL),(2,1,1,NULL),(3,2,1,NULL)`)
	oldBefore := readStartupFixture(t, f.oldDB)
	targetBefore := readStartupFixture(t, f.targetDB)
	app, c := prepareStartupUpgrade(f.paths)
	defer app.Close()
	if state := c.snapshot(); state.Phase != "blocked" || state.Kind != "newer" {
		t.Fatal(state)
	}
	assertStartupFileUnchanged(t, f.oldDB, oldBefore)
	assertStartupFileUnchanged(t, f.targetDB, targetBefore)
	if _, err := os.Stat(f.marker); err != nil {
		t.Fatal("version refusal discarded recovery evidence", err)
	}
}
func TestUpgradeActivationFailureRetainsVerifiedBackup(t *testing.T) {
	_, app, c := prepareUpgradeFixture(t, true)
	c.activate = func() error { return errors.New("settings initialization failed") }
	state := (&StartupService{control: c}).Confirm()
	if state.Phase != "blocked" || state.BackupPath == "" {
		t.Fatal(state)
	}
	if got := rawUpgradeScalar(t, state.BackupPath, `SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'`); got != "0" {
		t.Fatal(got)
	}
	if err := app.database.WithRead(func(*sql.DB, string) error { t.Error("failed startup left database active"); return nil }); err == nil {
		t.Fatal("database was not closed")
	}
}

// Identical PRAGMA column/key output is insufficient: REPLACE/IGNORE can change
// write semantics, and extra CHECKs must not be accepted as a known old schema.
func TestUpgradeRejectsLegacyConstraintChangesBeforeConsent(t *testing.T) {
	for _, constraint := range []string{"UNIQUE ON CONFLICT REPLACE", "UNIQUE ON CONFLICT IGNORE", "UNIQUE CHECK(length(name)>0)"} {
		t.Run(constraint, func(t *testing.T) {
			p := startupPathsFixture(t)
			path := filepath.Join(p.defaultDir, "study.db")
			seedHistoricalStartupDatabase(t, path, 1, func(sql string) string {
				return strings.Replace(sql, "name TEXT NOT NULL UNIQUE", "name TEXT NOT NULL "+constraint, 1)
			})
			before := readStartupFixture(t, path)
			app, c := prepareStartupUpgrade(p)
			defer app.Close()
			if state := c.snapshot(); state.Phase != "blocked" || state.Kind != "unrecognized" {
				t.Fatal(state)
			}
			assertStartupFileUnchanged(t, path, before)
			if _, err := os.Stat(filepath.Join(p.defaultDir, "backups")); !os.IsNotExist(err) {
				t.Fatalf("unrecognized schema created backups: %v", err)
			}
		})
	}
}
