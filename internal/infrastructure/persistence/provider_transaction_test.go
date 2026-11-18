package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"modernc.org/sqlite"
)

type providerProbeExecutor struct {
	Executor
	before func(string) error
}

func (p providerProbeExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if p.before != nil {
		if err := p.before(query); err != nil {
			return nil, err
		}
	}
	return p.Executor.ExecContext(ctx, query, args...)
}
func runProviderProbe(d *Database, ctx context.Context, body string, before func(string) error) error {
	return d.WithAccess(ctx, func(a *Access) error {
		return a.WithTransactionProvider(ctx, func(db *sql.DB, run PinnedTransactionRunner) error {
			source := fstest.MapFS{"00001_probe.sql": &fstest.MapFile{Data: []byte("-- +goose Up\n" + body + "\n")}}
			p, err := goose.NewProvider(goose.DialectSQLite3, db, source, goose.WithDisableGlobalRegistry(true), goose.WithTransactionRunner(func(ctx context.Context, conn *sql.Conn, work func(database.DBTxConn) error) error {
				return run(ctx, conn, func(exec Executor) error { return work(providerProbeExecutor{Executor: exec, before: before}) })
			}))
			if err != nil {
				return err
			}
			_, err = p.UpTo(ctx, 1)
			return err
		})
	})
}
func TestProviderTransactionOutcomes(t *testing.T) {
	for _, outcome := range []string{"success", "sql_error", "validation_error", "panic", "initialization_panic", "cancel", "IOERR", "NOMEM"} {
		t.Run(outcome, func(t *testing.T) {
			d, db, _ := transactionTestDatabase(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			marker := &struct{}{}
			primary := errors.New("validation failed")
			if outcome == "IOERR" {
				primary = &stage5NativeFault{code: 10, name: "IOERR"}
			}
			if outcome == "NOMEM" {
				primary = &stage5NativeFault{code: 7, name: "NOMEM"}
			}
			body := `INSERT INTO connection_probe_parent VALUES(1); SELECT 1 /* host-probe */;`
			if outcome == "sql_error" {
				body += ` INSERT INTO connection_probe_parent VALUES(1);`
			}
			calls := 0
			var result error
			func() {
				if strings.Contains(outcome, "panic") {
					defer func() {
						if got := recover(); got != marker {
							t.Errorf("panic changed: %v", got)
						}
					}()
				}
				result = runProviderProbe(d, ctx, body, func(query string) error {
					if outcome == "initialization_panic" && strings.HasPrefix(query, "CREATE TABLE goose_db_version") {
						panic(marker)
					}
					if !strings.Contains(query, "host-probe") {
						return nil
					}
					calls++
					switch outcome {
					case "panic":
						panic(marker)
					case "cancel":
						cancel()
						return ctx.Err()
					case "validation_error", "IOERR", "NOMEM":
						return primary
					}
					return nil
				})
			}()
			want := 0
			if outcome == "success" {
				want = 1
				if result != nil {
					t.Fatal(result)
				}
			} else if !strings.Contains(outcome, "panic") && result == nil {
				t.Fatal("failure reported success")
			}
			if outcome == "validation_error" || outcome == "IOERR" || outcome == "NOMEM" {
				if !errors.Is(result, primary) {
					t.Fatalf("original error lost: %v", result)
				}
			}
			if calls > 1 {
				t.Fatal("retried work", calls)
			}
			if stats := db.Stats(); stats.InUse != 0 || stats.MaxOpenConnections != 1 {
				t.Fatalf("connection leaked: %+v", stats)
			}
			assertProbeCount(t, db, "connection_probe_parent", want)
			if outcome != "initialization_panic" {
				var version int
				if err := db.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil || version != want {
					t.Fatalf("version %d %v", version, err)
				}
			}
			if err := d.WithAccess(context.Background(), func(a *Access) error {
				return a.WithTx(context.Background(), func(exec Executor) error { return insertTransactionProbe(context.Background(), exec, 2) })
			}); err != nil {
				t.Fatalf("subsequent transaction: %v", err)
			}
		})
	}
}
func TestProviderTransactionRollbackFailureIsolation(t *testing.T) {
	primary := errors.New("upgrade SQL failed")
	cleanup := errors.New("rollback failed")
	faults := &transactionFaultConnector{rollbackErr: cleanup}
	d, db := transactionFaultDatabase(t, faults)
	err := runProviderProbe(d, context.Background(), `INSERT INTO connection_probe_parent VALUES(1); SELECT 1 /* host-probe */;`, func(q string) error {
		if strings.Contains(q, "host-probe") {
			return primary
		}
		return nil
	})
	if !errors.Is(err, primary) || !errors.Is(err, cleanup) || d.pauseErr == nil {
		t.Fatalf("errors/isolation: %v %v", err, d.pauseErr)
	}
	if stats := db.Stats(); stats.InUse != 0 || stats.OpenConnections != 0 {
		t.Fatalf("bad connection retained: %+v", stats)
	}
	if faults.rollbacks.Load() != 1 {
		t.Fatal("rollback count", faults.rollbacks.Load())
	}
	if err := d.WithRead(func(*sql.DB, string) error { t.Error("paused database entered"); return nil }); err == nil {
		t.Fatal("database not paused")
	}
}
func TestProviderInitializationFailureDoesNotRetry(t *testing.T) {
	for _, boundary := range []string{"begin", "commit"} {
		t.Run(boundary, func(t *testing.T) {
			primary := errors.New("injected " + boundary + " failure")
			faults := &transactionFaultConnector{}
			if boundary == "begin" {
				faults.beginErr = primary
			} else {
				faults.commitErr = primary
			}
			d, db := transactionFaultDatabase(t, faults)
			calls := 0
			err := runProviderProbe(d, context.Background(), `INSERT INTO connection_probe_parent VALUES(1);`, func(string) error { calls++; return nil })
			if !errors.Is(err, primary) || d.pauseErr == nil {
				t.Fatalf("%v / %v", err, d.pauseErr)
			}
			if faults.closes.Load() != 1 || db.Stats().OpenConnections != 0 {
				t.Fatalf("connection not discarded %+v", db.Stats())
			}
			if calls > 2 {
				t.Fatal("initialization retried", calls)
			}
		})
	}
}
func TestProviderScopeCannotEscapeOrNest(t *testing.T) {
	d, _, _ := transactionTestDatabase(t)
	ctx := context.Background()
	var escaped PinnedTransactionRunner
	if err := d.WithAccess(ctx, func(a *Access) error {
		return a.WithTransactionProvider(ctx, func(db *sql.DB, run PinnedTransactionRunner) error {
			escaped = run
			if err := a.WithTx(ctx, func(Executor) error { t.Error("nested transaction entered"); return nil }); !errors.Is(err, ErrTransactionNested) {
				return fmt.Errorf("nested transaction: %v", err)
			}
			if _, err := a.Queries(ctx).Exec("SELECT 1"); !errors.Is(err, ErrTransactionNested) {
				return fmt.Errorf("outer query: %v", err)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := escaped(ctx, nil, func(Executor) error { return nil }); !errors.Is(err, ErrAccessExpired) {
		t.Fatal(err)
	}
}

func TestProviderRealResourceFailures(t *testing.T) {
	for _, failure := range []string{"FULL", "NOMEM"} {
		t.Run(failure, func(t *testing.T) {
			d, db, path := transactionTestDatabase(t)
			store, err := database.NewStore(database.DialectSQLite3, "goose_db_version")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.CreateVersionTable(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			if err := store.Insert(context.Background(), db, database.InsertRequest{Version: 0}); err != nil {
				t.Fatal(err)
			}
			body := `INSERT INTO connection_probe_parent VALUES(1);`
			code := 13
			if failure == "FULL" {
				var pages, limit int
				if err := db.QueryRow(`PRAGMA page_count`).Scan(&pages); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(fmt.Sprintf(`PRAGMA max_page_count=%d`, pages+4)).Scan(&limit); err != nil {
					t.Fatal(err)
				}
				body += `CREATE TABLE upgrade_full_probe(value BLOB); INSERT INTO upgrade_full_probe VALUES(zeroblob(1048576));`
			} else {
				code = 7
				ctx := context.Background()
				conn, err := db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				_, limitErr := sqlite.Limit(conn, 5, 100)
				if err := errors.Join(limitErr, conn.Close()); err != nil {
					t.Fatal(err)
				}
				queries := []string{}
				for i := 0; i < 60; i++ {
					queries = append(queries, fmt.Sprintf("SELECT %d", i))
				}
				body += strings.Join(queries, " UNION ALL ") + ";"
			}
			err = runProviderProbe(d, context.Background(), body, nil)
			requireSQLiteCode(t, err, code)
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			assertProbeCount(t, reopened, "connection_probe_parent", 0)
			var version int
			if err := reopened.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil || version != 0 {
				t.Fatalf("version %d %v", version, err)
			}
			requireStage5Integrity(t, reopened)
		})
	}
}
