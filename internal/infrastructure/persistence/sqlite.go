package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Open 运行现有初始化流程，包括首次创建数据库。
func Open(path string) (*sql.DB, error) {
	return openSQLite(path, true, initializeSchema)
}

// OpenForStartup 打开一个已选定且已存在的数据库，但不创建文件。仅允许初始化完整的 StudyFlow schema 或经过验证的历史 schema。
func OpenForStartup(path string) (*sql.DB, error) {
	return openSQLite(path, false, initializeExistingForStartup)
}

// OpenUnprepared opens only the connection. Startup must inspect compatibility
// before initializing schema or starting any business services.
func OpenUnprepared(path string, create bool) (*sql.DB, error) {
	return openSQLite(path, create, func(*sql.DB) error { return nil })
}

// WithStartupAccess borrows a pool not yet handed to the running application.
// The caller retains ownership and must close it on every failed startup.
func WithStartupAccess(ctx context.Context, db *sql.DB, use func(*Access) error) error {
	owner := &Database{db: db}
	return owner.WithAccess(ctx, use)
}

// RecognizeHistoricalBaseline accepts only the three complete known five-table
// profiles. No missing-table guessing or writes occur during recognition.
func RecognizeHistoricalBaseline(ctx context.Context, exec Executor) (bool, error) {
	return matchesLegacyStudyFlowSchema(BindQueries(ctx, exec))
}

// UpgradeHistoricalBaseline runs only inside the caller's baseline transaction.
func UpgradeHistoricalBaseline(ctx context.Context, exec Executor) error {
	ok, err := RecognizeHistoricalBaseline(ctx, exec)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("unknown historical baseline")
	}
	return initializeSchemaQueries(BindQueries(ctx, exec))
}

// OpenExisting 打开一个已准备好的数据库，不执行迁移。
func OpenExisting(path string) (*sql.DB, error) {
	return openSQLite(path, false, validateExisting)
}

func openSQLite(path string, allowCreate bool, prepare func(*sql.DB) error) (result *sql.DB, resultErr error) {
	params := url.Values{"_pragma": {"foreign_keys(ON)"}}
	dsn := ":memory:?" + params.Encode()
	if path != ":memory:" || !allowCreate {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		absolute = filepath.ToSlash(absolute)
		if !strings.HasPrefix(absolute, "/") {
			absolute = "/" + absolute
		}
		if !allowCreate {
			params.Set("mode", "rw")
		}
		dsn = "file://" + (&url.URL{Path: absolute}).EscapedPath() + "?" + params.Encode()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Startup has no external owner yet. Close on every failed return and on
	// panic, after the transaction runner has finished its own cleanup.
	defer func() {
		if result == nil {
			resultErr = closeAfterError(resultErr, func() error {
				closeErr := db.Close()
				if closeErr != nil {
					log.Printf("close unaccepted SQLite database: %v", closeErr)
				}
				return closeErr
			})
		}
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := verifyForeignKeys(db); err != nil {
		return nil, err
	}
	if err := prepare(db); err != nil {
		return nil, err
	}
	return db, nil
}

func verifyForeignKeys(db *sql.DB) error {
	var enabled int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return errors.New("SQLite foreign keys are not enabled")
	}
	return nil
}

func validateExisting(db *sql.DB) error {
	if err := checkSQLiteIntegrity(BindQueries(context.Background(), db)); err != nil {
		return err
	}
	return validateStudyFlowSchema(BindQueries(context.Background(), db))
}

func checkSQLiteIntegrity(db *Queries) error {
	var result string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("SQLite integrity check failed: %s", result)
	}
	return nil
}

var errMissingStudyFlowTables = errors.New("SQLite database is missing StudyFlow tables")

func validateStudyFlowSchema(db *Queries) error {
	var tables int
	err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name IN (
		'sessions','activity_logs','settings','daily_reviews','daily_review_items'
	)`).Scan(&tables)
	if err != nil {
		return err
	}
	if tables != 5 {
		return errMissingStudyFlowTables
	}

	var history bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name='plan_versions')`).Scan(&history); err != nil {
		return err
	}
	// 解析限定列名，但不扫描或修改任何行。
	versionQueries := []string{
		`SELECT t.id, t.name, t.color, t.created_at, t.archived, t.deleted, t.sort_order FROM subjects AS t LIMIT 0`,
		`SELECT t.id, t.subject_id, t.start_time, t.duration_minutes, t.weekdays, t.active,
		     t.color, t.deleted, t.break_minutes, t.note FROM plans AS t LIMIT 0`,
		`SELECT t.day, t.targets_json FROM daily_plan_targets AS t LIMIT 0`,
	}
	if history {
		versionQueries = []string{
			`SELECT t.id,t.subject_id,t.effective_day,t.name,t.color,t.sort_order,t.created_at,t.status
			 FROM subject_versions AS t LIMIT 0`,
			`SELECT t.id,t.kind,t.plan_id,t.subject_id,t.effective_day,t.start_time,t.duration_minutes,
			     t.break_minutes,t.weekdays,t.color,t.note,t.status,t.legacy_targets_json,t.adoption_day
			 FROM plan_versions AS t LIMIT 0`,
			`SELECT t.plan_version_id FROM sessions AS t LIMIT 0`,
		}
	} else {
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name IN
			('subjects','plans','daily_plan_targets')`).Scan(&tables); err != nil {
			return err
		}
		if tables != 3 {
			return errMissingStudyFlowTables
		}
	}
	for _, query := range append(versionQueries,
		`SELECT t.id, t.subject_id, t.plan_id, t.start_at, t.planned_end_at, t.end_at, t.paused_at,
		        t.resumed_at, t.duration_seconds, t.source, t.note, t.deleted, t.break_started_at,
		        t.break_notified, t.break_used_seconds
		 FROM sessions AS t LIMIT 0`,
		`SELECT t.id, t.happened_at, t.content FROM activity_logs AS t LIMIT 0`,
		`SELECT t.key, t.value FROM settings AS t LIMIT 0`,
		`SELECT t.day, t.mode, t.threshold, t.planned_seconds, t.actual_seconds, t.submitted_at
		 FROM daily_reviews AS t LIMIT 0`,
		`SELECT t.id, t.day, t.subject_id, t.subject_name, t.planned_seconds, t.actual_seconds, t.reason
		 FROM daily_review_items AS t LIMIT 0`,
	) {
		rows, err := db.Query(query)
		if err != nil {
			return fmt.Errorf("SQLite database has invalid StudyFlow columns: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	var goalsTable int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='stage_goals'`).Scan(&goalsTable); err != nil {
		return err
	}
	if goalsTable == 1 {
		rows, err := db.Query(`SELECT t.id,t.title,t.deadline,t.note,t.completed_at,t.created_at,t.updated_at,t.deleted
			FROM stage_goals AS t LIMIT 0`)
		if err != nil {
			return fmt.Errorf("SQLite database has invalid StudyFlow goal columns: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Initialization owns the unopened application's sole pool. Use the same
// transaction runner as business operations, including uncertain-commit cleanup.
func initializeAtomically(db *sql.DB, work func(*Queries) error) error {
	owner := &Database{db: db}
	ctx := context.Background()
	return owner.WithAccess(ctx, func(a *Access) error {
		return a.Queries(ctx).WithTx(work)
	})
}

func initializeExistingForStartup(db *sql.DB) error {
	return initializeAtomically(db, func(q *Queries) error {
		if err := checkSQLiteIntegrity(q); err != nil {
			return err
		}
		if err := validateStudyFlowSchema(q); err != nil {
			if !errors.Is(err, errMissingStudyFlowTables) {
				return err
			}
			legacy, checkErr := matchesLegacyStudyFlowSchema(q)
			if checkErr != nil {
				return fmt.Errorf("check historical StudyFlow schema: %w", checkErr)
			}
			if !legacy {
				return fmt.Errorf("unrecognized or incomplete StudyFlow schema: %w", err)
			}
		}
		if err := initializeSchemaQueries(q); err != nil {
			return fmt.Errorf("StudyFlow startup migration failed: %w", err)
		}
		if err := validateStudyFlowSchema(q); err != nil {
			return fmt.Errorf("validate StudyFlow schema after startup migration: %w", err)
		}
		return nil
	})
}

type legacyColumn struct {
	name, sqlType, defaultSQL string
	notNull, primaryKey       bool
}

func matchesLegacyStudyFlowSchema(db *Queries) (bool, error) {
	var objects, ordinaryTables int
	err := db.QueryRow(`
		SELECT (SELECT count(*) FROM main.sqlite_schema WHERE name NOT GLOB 'sqlite_*'),
		       (SELECT count(*) FROM pragma_table_list
		        WHERE schema='main' AND type='table' AND wr=0 AND strict=0
		          AND name IN ('subjects','plans','sessions','activity_logs','settings'))`).
		Scan(&objects, &ordinaryTables)
	if err != nil || objects != 5 || ordinaryTables != 5 {
		return false, err
	}

	var subjectColumns int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_xinfo('subjects', 'main')`).Scan(&subjectColumns); err != nil {
		return false, err
	}
	subjects := []legacyColumn{
		{name: "id", sqlType: "INTEGER", primaryKey: true},
		{name: "name", sqlType: "TEXT", notNull: true},
		{name: "color", sqlType: "TEXT", notNull: true, defaultSQL: "'#7668e8'"},
		{name: "created_at", sqlType: "TEXT", notNull: true},
	}
	plans := []legacyColumn{
		{name: "id", sqlType: "INTEGER", primaryKey: true},
		{name: "subject_id", sqlType: "INTEGER", notNull: true},
		{name: "start_time", sqlType: "TEXT", notNull: true},
		{name: "duration_minutes", sqlType: "INTEGER", notNull: true},
		{name: "weekdays", sqlType: "TEXT", notNull: true, defaultSQL: "'1,2,3,4,5,6,0'"},
		{name: "active", sqlType: "INTEGER", notNull: true, defaultSQL: "1"},
	}
	sessions := []legacyColumn{
		{name: "id", sqlType: "INTEGER", primaryKey: true},
		{name: "subject_id", sqlType: "INTEGER", notNull: true},
		{name: "start_at", sqlType: "TEXT", notNull: true},
		{name: "end_at", sqlType: "TEXT"},
		{name: "duration_seconds", sqlType: "INTEGER", notNull: true, defaultSQL: "0"},
		{name: "source", sqlType: "TEXT", notNull: true, defaultSQL: "'timer'"},
		{name: "note", sqlType: "TEXT", notNull: true, defaultSQL: "''"},
	}
	archived := legacyColumn{name: "archived", sqlType: "INTEGER", notNull: true, defaultSQL: "0"}
	// 将三种观察到的历史结构作为整体匹配；不得混用不同结构的列，也不得从部分 schema 推断缺失的表。
	switch subjectColumns {
	case 4: // study-legacy.db
	case 5: // exam_notify-legacy
		subjects = append(subjects, archived)
	case 6: // study.db
		subjects = append(subjects, archived, legacyColumn{name: "deleted", sqlType: "INTEGER", notNull: true, defaultSQL: "0"})
		plans = append(plans, legacyColumn{name: "color", sqlType: "TEXT", notNull: true, defaultSQL: "''"})
		sessions = append(sessions,
			legacyColumn{name: "plan_id", sqlType: "INTEGER"},
			legacyColumn{name: "paused_at", sqlType: "TEXT"},
			legacyColumn{name: "resumed_at", sqlType: "TEXT"})
	default:
		return false, nil
	}
	for _, table := range []struct {
		name                  string
		columns               []legacyColumn
		subjectForeignKey     bool
		indexOrigin, indexKey string
	}{
		{name: "subjects", columns: subjects, indexOrigin: "u", indexKey: "name"},
		{name: "plans", columns: plans, subjectForeignKey: true},
		{name: "sessions", columns: sessions, subjectForeignKey: true},
		{name: "activity_logs", columns: []legacyColumn{
			{name: "id", sqlType: "INTEGER", primaryKey: true},
			{name: "happened_at", sqlType: "TEXT", notNull: true},
			{name: "content", sqlType: "TEXT", notNull: true},
		}},
		{name: "settings", indexOrigin: "pk", indexKey: "key", columns: []legacyColumn{
			{name: "key", sqlType: "TEXT", primaryKey: true},
			{name: "value", sqlType: "TEXT", notNull: true},
		}},
	} {
		if matches, err := matchesLegacyColumns(db, table.name, table.columns); err != nil || !matches {
			return false, err
		}
		if matches, err := matchesLegacyKeys(db, table.name, table.subjectForeignKey, table.indexOrigin, table.indexKey); err != nil || !matches {
			return false, err
		}
	}
	return true, nil
}

func matchesLegacyColumns(db *Queries, table string, expected []legacyColumn) (bool, error) {
	rows, err := db.Query(`SELECT name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo(?, 'main')`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	byName := make(map[string]legacyColumn, len(expected))
	for _, column := range expected {
		byName[column.name] = column
	}
	var seen int
	for rows.Next() {
		var name, sqlType string
		var defaultSQL sql.NullString
		var notNull bool
		var pk, hidden int
		if err := rows.Scan(&name, &sqlType, &notNull, &defaultSQL, &pk, &hidden); err != nil {
			return false, err
		}
		want, ok := byName[name]
		if !ok || !strings.EqualFold(sqlType, want.sqlType) || notNull != want.notNull ||
			pk > 1 || (pk == 1) != want.primaryKey || hidden != 0 ||
			!matchesLegacyDefault(defaultSQL, want.defaultSQL) {
			return false, nil
		}
		seen++
	}
	return seen == len(expected), rows.Err()
}

// 观察到的默认值都是简单字面量。仅规范格式，不评估从现有数据库读取的任意 SQL 表达式。
func matchesLegacyDefault(actual sql.NullString, expected string) bool {
	if !actual.Valid || expected == "" {
		return !actual.Valid && expected == ""
	}
	value := strings.TrimSpace(actual.String)
	for len(value) >= 2 && value[0] == '(' && value[len(value)-1] == ')' {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if value == expected {
		return true
	}
	if expected[0] == '\'' {
		return len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' &&
			value[1:len(value)-1] == expected[1:len(expected)-1]
	}
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && strconv.FormatInt(number, 10) == expected
}

func matchesLegacyKeys(db *Queries, table string, subjectFK bool, indexOrigin, indexKey string) (bool, error) {
	var foreignKeys, matchingFKs, indexes, matchingIndexes int
	err := db.QueryRow(`
		SELECT (SELECT count(*) FROM pragma_foreign_key_list(?, 'main')),
		       (SELECT count(*) FROM pragma_foreign_key_list(?, 'main')
		        WHERE "table"='subjects' AND "from"='subject_id' AND "to"='id'
		          AND on_update='NO ACTION' AND on_delete='NO ACTION' AND match='NONE'),
		       (SELECT count(*) FROM pragma_index_list(?, 'main')),
		       (SELECT count(*) FROM pragma_index_list(?, 'main') AS i
		        WHERE i."unique"=1 AND i.partial=0 AND i.origin=?
		          AND (SELECT count(*) FROM pragma_index_xinfo(i.name, 'main') WHERE "key"=1)=1
		          AND EXISTS (SELECT 1 FROM pragma_index_xinfo(i.name, 'main')
		                      WHERE "key"=1 AND name=? AND coll='BINARY' AND "desc"=0))`,
		table, table, table, table, indexOrigin, indexKey).
		Scan(&foreignKeys, &matchingFKs, &indexes, &matchingIndexes)
	if err != nil {
		return false, err
	}
	expectedFKs, expectedIndexes := 0, 0
	if subjectFK {
		expectedFKs = 1
	}
	if indexOrigin != "" {
		expectedIndexes = 1
	}
	return foreignKeys == expectedFKs && matchingFKs == expectedFKs &&
		indexes == expectedIndexes && matchingIndexes == expectedIndexes, nil
}

// sqliteCloseError 标识一次失败的关闭，同时不掩盖主错误。
type sqliteCloseError struct{ cause error }

func (e *sqliteCloseError) Error() string { return fmt.Sprintf("close SQLite database: %v", e.cause) }
func (e *sqliteCloseError) Unwrap() error { return e.cause }

func closeAfterError(primary error, closeConnection func() error) error {
	if err := closeConnection(); err != nil {
		return errors.Join(primary, &sqliteCloseError{cause: err})
	}
	return primary
}

func initializeSchema(db *sql.DB) error {
	return initializeAtomically(db, func(q *Queries) error {
		if err := initializeSchemaQueries(q); err != nil {
			return err
		}
		return validateStudyFlowSchema(q)
	})
}

func initializeSchemaQueries(db *Queries) error {
	var err error
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL DEFAULT '#007AFF', created_at TEXT NOT NULL, archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0, sort_order INTEGER)`,
		`CREATE TABLE IF NOT EXISTS plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL DEFAULT '1,2,3,4,5,6,0', active INTEGER NOT NULL DEFAULT 1, color TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0, break_minutes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS sessions (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), plan_id INTEGER, start_at TEXT NOT NULL, planned_end_at TEXT, end_at TEXT, paused_at TEXT, resumed_at TEXT, duration_seconds INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'timer', note TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0, break_started_at TEXT, break_notified INTEGER NOT NULL DEFAULT 0, break_used_seconds INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS activity_logs (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, content TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS daily_reviews (day TEXT PRIMARY KEY, mode TEXT NOT NULL, threshold INTEGER NOT NULL, planned_seconds INTEGER NOT NULL, actual_seconds INTEGER NOT NULL, submitted_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS daily_review_items (id INTEGER PRIMARY KEY, day TEXT NOT NULL REFERENCES daily_reviews(day) ON DELETE CASCADE, subject_id INTEGER, subject_name TEXT NOT NULL, planned_seconds INTEGER NOT NULL, actual_seconds INTEGER NOT NULL, reason TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS daily_plan_targets (day TEXT PRIMARY KEY, targets_json TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS stage_goals (id INTEGER PRIMARY KEY, title TEXT NOT NULL, deadline TEXT NOT NULL, note TEXT NOT NULL DEFAULT '', completed_at TEXT, accepted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, deleted INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, s := range stmts {
		if _, err = db.Exec(s); err != nil {
			return err
		}
	}
	if _, err = db.Exec(`INSERT INTO settings(key,value) VALUES('dailyReviewStartDate',?) ON CONFLICT(key) DO NOTHING`, time.Now().Format("2006-01-02")); err != nil {
		return err
	}
	if _, err = db.Exec(`INSERT INTO settings(key,value) VALUES('dailyReviewEnabled','0') ON CONFLICT(key) DO NOTHING`); err != nil {
		return err
	}
	rows, err := db.Query(`PRAGMA table_info(subjects)`)
	if err != nil {
		return err
	}
	archivedColumn, deletedColumn := false, false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var def any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "archived" {
			archivedColumn = true
		}
		if name == "deleted" {
			deletedColumn = true
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if !archivedColumn {
		if _, err = db.Exec(`ALTER TABLE subjects ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !deletedColumn {
		if _, err = db.Exec(`ALTER TABLE subjects ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	rows, err = db.Query(`PRAGMA table_info(plans)`)
	if err != nil {
		return err
	}
	planColorColumn := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var def any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "color" {
			planColorColumn = true
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if !planColorColumn {
		if _, err = db.Exec(`ALTER TABLE plans ADD COLUMN color TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	var planIDColumn int
	if err = db.QueryRow(`SELECT count(*) FROM pragma_table_info('sessions') WHERE name='plan_id'`).Scan(&planIDColumn); err != nil {
		return err
	}
	if planIDColumn == 0 {
		if _, err = db.Exec(`ALTER TABLE sessions ADD COLUMN plan_id INTEGER`); err != nil {
			return err
		}
	}
	for _, column := range []string{"paused_at", "resumed_at"} {
		var exists int
		if err = db.QueryRow(`SELECT count(*) FROM pragma_table_info('sessions') WHERE name=?`, column).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err = db.Exec(`ALTER TABLE sessions ADD COLUMN ` + column + ` TEXT`); err != nil {
				return err
			}
		}
	}
	for _, table := range []string{"plans", "sessions"} {
		var exists int
		if err = db.QueryRow(`SELECT count(*) FROM pragma_table_info('` + table + `') WHERE name='deleted'`).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
		}
	}
	for _, migration := range []struct{ table, column, kind string }{
		{"subjects", "sort_order", "INTEGER"},
		{"plans", "break_minutes", "INTEGER NOT NULL DEFAULT 0"},
		{"plans", "note", "TEXT NOT NULL DEFAULT ''"},
		{"sessions", "break_started_at", "TEXT"},
		{"sessions", "break_notified", "INTEGER NOT NULL DEFAULT 0"},
		{"sessions", "break_used_seconds", "INTEGER NOT NULL DEFAULT 0"},
		{"sessions", "planned_end_at", "TEXT"},
		{"stage_goals", "accepted_at", "TEXT"},
	} {
		var exists int
		if err = db.QueryRow(`SELECT count(*) FROM pragma_table_info('`+migration.table+`') WHERE name=?`, migration.column).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err = db.Exec(`ALTER TABLE ` + migration.table + ` ADD COLUMN ` + migration.column + ` ` + migration.kind); err != nil {
				return err
			}
		}
	}
	// 历史已完成目标直接视为“已验收且完成”。
	if _, err = db.Exec(`UPDATE stage_goals SET accepted_at=completed_at WHERE completed_at IS NOT NULL AND accepted_at IS NULL`); err != nil {
		return err
	}
	// 旧版允许补录不足 10 分钟，界面却隐藏这些记录。沿用计时学习的
	// 软删除规则使其不再计入统计或占用时间，保留原始行便于恢复。
	if _, err = db.Exec(`UPDATE sessions SET deleted=1 WHERE deleted=0 AND source='manual' AND end_at IS NOT NULL AND duration_seconds<600`); err != nil {
		return err
	}
	// 将原先的单一颜色默认值替换为新调色板的分布。
	legacyRows, err := db.Query(`SELECT id FROM subjects WHERE color='#7668e8' ORDER BY id`)
	if err != nil {
		return err
	}
	legacyIDs := []int64{}
	for legacyRows.Next() {
		var id int64
		if err = legacyRows.Scan(&id); err != nil {
			legacyRows.Close()
			return err
		}
		legacyIDs = append(legacyIDs, id)
	}
	if err := errors.Join(legacyRows.Err(), legacyRows.Close()); err != nil {
		return err
	}
	palette := []string{"#007AFF", "#34C759", "#FF9500", "#AF52DE", "#5AC8FA", "#FF3B30", "#5856D6", "#FF2D55", "#A2845E", "#00C7BE"}
	for i, id := range legacyIDs {
		if _, err = db.Exec(`UPDATE subjects SET color=? WHERE id=?`, palette[i%len(palette)], id); err != nil {
			return err
		}
	}
	return nil
}

// UpgradeHistoricalGoalSchema is used only after upgrade's full frozen profile
// check has identified the missing goal table/acceptance column, inside WithTx.
func UpgradeHistoricalGoalSchema(ctx context.Context, exec Executor) error {
	return initializeSchemaQueries(BindQueries(ctx, exec))
}
