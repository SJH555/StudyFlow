package upgrade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

type Manager struct {
	source          fs.FS
	target          int64
	validateVersion func(context.Context, persistence.Executor, int64) error
}
type State struct {
	Version, Target  int64
	Fresh, Untracked bool
	Historical       bool
	HistoricalGoals  bool
	Versioned        bool
}

func (s State) NeedsUpgrade() bool { return s.Fresh || s.Untracked || s.Version < s.Target }

func newManager(source fs.FS, target int64) (*Manager, error) {
	if target < 1 {
		return nil, errors.New("invalid target database version")
	}
	// Only ship the scripts up to this APP's explicitly declared target.
	files, err := fs.Glob(source, "*.sql")
	if err != nil {
		return nil, err
	}
	selected := mapFS{}
	seen := map[int64]bool{}
	for _, name := range files {
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("invalid upgrade filename %s", name)
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || v < 1 {
			return nil, fmt.Errorf("invalid upgrade version %s", name)
		}
		if v > target {
			continue
		}
		if seen[v] {
			return nil, fmt.Errorf("duplicate upgrade version %d", v)
		}
		seen[v] = true
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return nil, err
		}
		if err := checkScript(string(data)); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		selected[name] = data
	}
	for v := int64(1); v <= target; v++ {
		if !seen[v] {
			return nil, fmt.Errorf("missing upgrade SQL for version %d", v)
		}
	}
	return &Manager{source: selected, target: target}, nil
}

// Inspect is read-only. In particular, no Goose Provider method which might
// create its version table is called before consent and backup.
func (m *Manager) Inspect(ctx context.Context, a *persistence.Access) (state State, err error) {
	err = a.WithTransactionProvider(ctx, func(db *sql.DB, _ persistence.PinnedTransactionRunner) error {
		var e error
		state, e = m.inspect(ctx, db)
		return e
	})
	return
}
func (m *Manager) inspect(ctx context.Context, db persistence.Executor) (State, error) {
	state := State{Target: m.target}
	if err := checkIntegrity(ctx, db); err != nil {
		return state, err
	}
	var ledger int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_schema WHERE type='table' AND name=?`, versionTable).Scan(&ledger); err != nil {
		return state, err
	}
	if ledger == 0 {
		var objects int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_schema WHERE name NOT GLOB 'sqlite_*'`).Scan(&objects); err != nil {
			return state, err
		}
		if objects == 0 {
			state.Fresh = true
			return state, nil
		}
		if err := recognizeBaseline(ctx, db); err != nil {
			goals := recognizeHistoricalGoals(ctx, db) == nil
			historical, checkErr := recognizeLegacyBaseline(ctx, db)
			if checkErr != nil {
				return state, checkErr
			}
			if !historical && !goals {
				return state, err
			}
			state.Historical = true
			state.HistoricalGoals = goals
		}
		state.Version = 1
		state.Untracked = true
		return state, nil
	}
	state.Versioned = true
	rows, err := db.QueryContext(ctx, `SELECT version_id,is_applied FROM goose_db_version ORDER BY id`)
	if err != nil {
		return state, fmt.Errorf("%w: %w", ErrInvalidHistory, err)
	}
	var count int64
	for rows.Next() {
		var version, applied int64
		if err := rows.Scan(&version, &applied); err != nil {
			rows.Close()
			return state, err
		}
		if version != count || applied != 1 {
			rows.Close()
			return state, ErrInvalidHistory
		}
		state.Version = version
		count++
	}
	if err := closeRows(rows); err != nil {
		return state, err
	}
	if count == 0 {
		return state, ErrInvalidHistory
	}
	if state.Version > m.target {
		return state, fmt.Errorf("%w: database %d, application supports %d", ErrNewerDatabase, state.Version, m.target)
	}
	if state.Version == 0 {
		var objects int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_schema WHERE name NOT GLOB 'sqlite_*' AND name != ?`, versionTable).Scan(&objects); err != nil {
			return state, err
		}
		if objects != 0 {
			return state, ErrUnknownSchema
		}
		state.Fresh = true
	}
	if state.Version == 1 {
		if err := recognizeBaseline(ctx, db); err != nil {
			return state, err
		}
	}
	if m.validateVersion != nil {
		if err := m.validateVersion(ctx, db, state.Version); err != nil {
			return state, fmt.Errorf("%w: %w", ErrUnknownSchema, err)
		}
	}
	return state, nil
}

func validateScheduleHistory(ctx context.Context, db persistence.Executor, version int64) error {
	if version < 2 {
		return nil
	}
	for _, query := range []string{
		`SELECT id,subject_id,effective_day,name,color,sort_order,created_at,status FROM subject_versions LIMIT 0`,
		`SELECT id,kind,plan_id,subject_id,effective_day,start_time,duration_minutes,break_minutes,weekdays,color,note,status,legacy_targets_json,adoption_day FROM plan_versions LIMIT 0`,
		`SELECT plan_version_id FROM sessions LIMIT 0`,
	} {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		if err := closeRows(rows); err != nil {
			return err
		}
	}
	var reviewStatus bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pragma_table_info('daily_reviews') WHERE name='status')`).Scan(&reviewStatus); err != nil {
		return err
	}
	if !reviewStatus {
		return errors.New("版本 2 缺少每日追因结果字段，请使用保留的版本 1 备份进行升级")
	}
	var staleTables bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.sqlite_schema WHERE type='table'
		AND name IN ('subjects','plans','daily_plan_targets','schedule_history'))`).Scan(&staleTables); err != nil {
		return err
	}
	if staleTables {
		return errors.New("database still contains superseded schedule tables")
	}
	var orphaned bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM plan_versions p WHERE p.kind='recurring'
		AND NOT EXISTS(SELECT 1 FROM subject_versions s WHERE s.subject_id=p.subject_id)
		UNION ALL
		SELECT 1 FROM sessions t WHERE
		NOT EXISTS(SELECT 1 FROM subject_versions s WHERE s.subject_id=t.subject_id)
		OR (t.plan_version_id IS NOT NULL AND NOT EXISTS(
			SELECT 1 FROM plan_versions p WHERE p.id=t.plan_version_id AND p.kind='recurring'
			AND p.plan_id=t.plan_id))
	)`).Scan(&orphaned); err != nil {
		return err
	}
	if orphaned {
		return errors.New("database has invalid references to schedule history")
	}
	var day string
	var adoption sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT effective_day,adoption_day FROM plan_versions WHERE kind='history_boundary'`).Scan(&day, &adoption); err != nil {
		return err
	}
	first, err := time.Parse(time.DateOnly, day)
	if err != nil || !adoption.Valid {
		return err
	}
	imported, err := time.Parse(time.DateOnly, adoption.String)
	if err == nil && !imported.AddDate(0, 0, 1).Equal(first) {
		return errors.New("schedule history starts on an invalid date")
	}
	return err
}

// Apply requires the caller to obtain upgrade consent and a verified backup
// first (unless Fresh). It must run before any business/background work starts.
// Each SQL file and its Goose version entry share our existing transaction core.
func (m *Manager) Apply(ctx context.Context, a *persistence.Access, expected State) error {
	current, err := m.Inspect(ctx, a)
	if err != nil {
		return err
	}
	if current != expected {
		return errors.New("database upgrade state changed; inspect again")
	}
	if !current.NeedsUpgrade() {
		return nil
	}
	// Validate Goose annotations before making the first write.
	if err := m.withProvider(ctx, a, func(p *goose.Provider) error { return p.ValidateSQLMigrations() }); err != nil {
		return err
	}
	if current.Untracked {
		store, err := database.NewStore(database.DialectSQLite3, versionTable)
		if err != nil {
			return err
		}
		if err := a.WithTx(ctx, func(tx persistence.Executor) error {
			// Convert only an explicitly recognized historical profile, after backup.
			if current.HistoricalGoals {
				if err := recognizeHistoricalGoals(ctx, tx); err != nil {
					return err
				}
				if err := persistence.UpgradeHistoricalGoalSchema(ctx, tx); err != nil {
					return err
				}
			} else if current.Historical {
				if err := persistence.UpgradeHistoricalBaseline(ctx, tx); err != nil {
					return err
				}
			}
			// Recheck the baseline inside the registration transaction.
			if err := recognizeBaseline(ctx, tx); err != nil {
				return err
			}
			if err := store.CreateVersionTable(ctx, tx); err != nil {
				return err
			}
			if err := store.Insert(ctx, tx, database.InsertRequest{Version: 0}); err != nil {
				return err
			}
			return store.Insert(ctx, tx, database.InsertRequest{Version: 1})
		}); err != nil {
			return err
		}
	}
	if err := m.withProvider(ctx, a, func(p *goose.Provider) error { _, err := p.UpTo(ctx, m.target); return err }); err != nil {
		return err
	}
	result, err := m.Inspect(ctx, a)
	if err != nil {
		return err
	}
	if result.NeedsUpgrade() || result.Version != m.target {
		return errors.New("database upgrade did not reach target version")
	}
	return nil
}
func (m *Manager) withProvider(ctx context.Context, a *persistence.Access, use func(*goose.Provider) error) error {
	return a.WithTransactionProvider(ctx, func(db *sql.DB, run persistence.PinnedTransactionRunner) error {
		p, err := goose.NewProvider(goose.DialectSQLite3, db, m.source,
			goose.WithDisableGlobalRegistry(true),
			goose.WithTransactionRunner(func(ctx context.Context, conn *sql.Conn, work func(database.DBTxConn) error) error {
				return run(ctx, conn, func(exec persistence.Executor) error { return work(exec) })
			}))
		if err != nil {
			return err
		}
		// Provider.Close would close the pool owned by persistence. Provider methods
		// release their own borrowed connection, including on error/panic.
		return use(p)
	})
}
func checkIntegrity(ctx context.Context, db persistence.Executor) error {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if result != "ok" {
			rows.Close()
			return fmt.Errorf("database integrity: %s", result)
		}
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	rows, err = db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	if rows.Next() {
		rows.Close()
		return errors.New("database has foreign key violations")
	}
	return closeRows(rows)
}

// ValidateReady protects candidate handoff without running upgrade scripts.
func (m *Manager) ValidateReady(ctx context.Context, exec persistence.Executor) error {
	state, err := m.inspect(ctx, exec)
	if err != nil {
		return err
	}
	if state.NeedsUpgrade() {
		return errors.New("database needs upgrade before it can be used")
	}
	return nil
}

// VerifySnapshot reads a backup while persistence owns its open/close lifecycle.
func (m *Manager) VerifySnapshot(ctx context.Context, exec persistence.Executor, expected State) error {
	state, err := m.inspect(ctx, exec)
	if err != nil {
		return err
	}
	if state != expected {
		return errors.New("backup does not match inspected database")
	}
	return nil
}
