package persistence

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func transactionTestDatabase(t *testing.T) (*Database, *sql.DB, string) {
	t.Helper()
	db, path := connectionProbeDB(t)
	d := &Database{db: db, path: path}
	t.Cleanup(func() { _ = d.Close() })
	return d, db, path
}

func insertTransactionProbe(ctx context.Context, executor Executor, id int) error {
	_, err := executor.ExecContext(ctx, `INSERT INTO connection_probe_parent VALUES(?)`, id)
	return err
}

func TestTransactionOutcomes(t *testing.T) {
	for _, outcome := range []string{"commit", "business_error", "sql_error", "panic", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			d, db, _ := transactionTestDatabase(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			businessErr := errors.New("injected business validation failure")
			panicValue := &struct{ message string }{"original panic"}
			var resultErr error
			func() {
				if outcome == "panic" {
					defer func() {
						if got := recover(); got != panicValue {
							t.Errorf("panic = %v, want original panic", got)
						}
					}()
				}
				resultErr = d.WithAccess(ctx, func(a *Access) error {
					return a.WithTx(ctx, func(executor Executor) error {
						if _, exposed := executor.(interface{ Commit() error }); exposed {
							t.Fatal("transaction commit exposed to business callback")
						}
						if err := insertTransactionProbe(ctx, executor, 1); err != nil {
							return err
						}
						switch outcome {
						case "business_error":
							return businessErr
						case "sql_error":
							return insertTransactionProbe(ctx, executor, 1)
						case "panic":
							panic(panicValue)
						case "cancel":
							cancel()
							// A callback returning nil must not cause cancelled work to commit.
							return nil
						}
						return nil
					})
				})
			}()
			wantCount := 0
			switch outcome {
			case "commit":
				wantCount = 1
				if resultErr != nil {
					t.Fatal(resultErr)
				}
			case "business_error":
				if !errors.Is(resultErr, businessErr) {
					t.Fatalf("business error lost: %v", resultErr)
				}
			case "sql_error":
				requireSQLiteCode(t, resultErr, 19)
			case "cancel":
				if !errors.Is(resultErr, context.Canceled) {
					t.Fatalf("cancellation lost: %v", resultErr)
				}
			}
			assertProbeCount(t, db, "connection_probe_parent", wantCount)
			if err := d.WithAccess(context.Background(), func(a *Access) error {
				return a.WithTx(context.Background(), func(executor Executor) error {
					return insertTransactionProbe(context.Background(), executor, 2)
				})
			}); err != nil {
				t.Fatalf("subsequent transaction: %v", err)
			}
			assertProbeCount(t, db, "connection_probe_parent", wantCount+1)
		})
	}
}

func TestTransactionCommitFailurePausesAllAccess(t *testing.T) {
	for _, failure := range []string{"busy", "deferred_foreign_key"} {
		t.Run(failure, func(t *testing.T) {
			d, db, path := transactionTestDatabase(t)
			if failure == "busy" {
				reader, err := OpenExisting(path)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				tx, err := reader.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				var count int
				if err := tx.QueryRow(`SELECT count(*) FROM connection_probe_parent`).Scan(&count); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			err := d.WithAccess(context.Background(), func(a *Access) error {
				return a.WithTx(context.Background(), func(executor Executor) error {
					calls++
					if failure == "deferred_foreign_key" {
						if _, err := executor.ExecContext(context.Background(), `PRAGMA defer_foreign_keys=ON`); err != nil {
							return err
						}
						_, err := executor.ExecContext(context.Background(), `INSERT INTO connection_probe_child VALUES(1,999)`)
						return err
					}
					return insertTransactionProbe(context.Background(), executor, 1)
				})
			})
			code := 5
			if failure == "deferred_foreign_key" {
				code = 19
			}
			requireSQLiteCode(t, err, code)
			if calls != 1 || !errors.Is(err, errDatabaseUnavailable) {
				t.Fatalf("calls=%d, result=%v", calls, err)
			}
			if stats := db.Stats(); stats.OpenConnections != 0 {
				t.Fatalf("unsafe connection retained: %+v", stats)
			}
			assertTransactionAccessBlocked(t, d)
		})
	}
}

func assertTransactionAccessBlocked(t *testing.T, d *Database) {
	t.Helper()
	for _, borrow := range []func(func(*sql.DB, string) error) error{d.WithRead, d.WithWrite} {
		if err := borrow(func(*sql.DB, string) error {
			t.Error("paused database admitted a borrower")
			return nil
		}); !errors.Is(err, errDatabaseUnavailable) {
			t.Fatalf("paused access: %v", err)
		}
	}
	if err := d.WithAccess(context.Background(), func(*Access) error {
		t.Error("paused database admitted a scope")
		return nil
	}); !errors.Is(err, errDatabaseUnavailable) {
		t.Fatalf("paused scope: %v", err)
	}
}

func TestTransactionScopeGuards(t *testing.T) {
	d, _, _ := transactionTestDatabase(t)
	var escaped *Access
	ctx := context.Background()
	if err := d.WithAccess(ctx, func(a *Access) error {
		escaped = a
		return a.WithTx(ctx, func(executor Executor) error {
			if err := a.WithTx(ctx, func(Executor) error { t.Error("nested callback ran"); return nil }); !errors.Is(err, ErrTransactionNested) {
				t.Errorf("nested transaction: %v", err)
			}
			if err := a.Queries(ctx).QueryRow(`SELECT 1`).Scan(new(int)); !errors.Is(err, ErrTransactionNested) {
				t.Errorf("outer queries available during transaction: %v", err)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := escaped.WithTx(ctx, func(Executor) error { t.Error("expired callback ran"); return nil }); !errors.Is(err, ErrAccessExpired) {
		t.Fatalf("expired transaction: %v", err)
	}
	if err := escaped.Queries(ctx).QueryRow(`SELECT 1`).Scan(new(int)); !errors.Is(err, ErrAccessExpired) {
		t.Fatalf("expired query access: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := d.WithAccess(cancelled, func(*Access) error { t.Error("cancelled scope ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled access: %v", err)
	}
}

func TestTransactionCancellationInterruptsQuery(t *testing.T) {
	d, db, _ := transactionTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := d.WithAccess(context.Background(), func(a *Access) error {
		return a.WithTx(ctx, func(executor Executor) error {
			if err := insertTransactionProbe(ctx, executor, 1); err != nil {
				return err
			}
			var sum int64
			return executor.QueryRowContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000000000) SELECT sum(x) FROM n`).Scan(&sum)
		})
	})
	if err == nil || ctx.Err() == nil {
		t.Fatalf("query not cancelled: result=%v, context=%v", err, ctx.Err())
	}
	assertProbeCount(t, db, "connection_probe_parent", 0)
	if err := d.WithAccess(context.Background(), func(a *Access) error {
		return a.WithTx(context.Background(), func(executor Executor) error {
			return insertTransactionProbe(context.Background(), executor, 2)
		})
	}); err != nil {
		t.Fatalf("after cancelled query: %v", err)
	}
}

// The following connector wraps real SQLite only to inject failures at the
// driver transaction boundary; real lock/constraint failures are tested above.
type transactionFaultConnector struct {
	path        string
	beginErr    error
	rollbackErr error
	commitErr   error
	closeErr    error
	afterCommit func()
	closes      atomic.Int32
	rollbacks   atomic.Int32
}

func (c *transactionFaultConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *transactionFaultConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.Driver().Open(c.path)
	if err != nil {
		return nil, err
	}
	return &transactionFaultConn{Conn: conn, faults: c}, nil
}

type transactionFaultConn struct {
	driver.Conn
	faults *transactionFaultConnector
}

func (c *transactionFaultConn) Begin() (driver.Tx, error) {
	if c.faults.beginErr != nil {
		return nil, c.faults.beginErr
	}
	tx, err := c.Conn.Begin()
	if err != nil {
		return nil, err
	}
	return &transactionFaultTx{Tx: tx, faults: c.faults}, nil
}
func (c *transactionFaultConn) Close() error {
	c.faults.closes.Add(1)
	return errors.Join(c.Conn.Close(), c.faults.closeErr)
}

type transactionFaultTx struct {
	driver.Tx
	faults *transactionFaultConnector
}

func (tx *transactionFaultTx) Rollback() error {
	tx.faults.rollbacks.Add(1)
	if tx.faults.rollbackErr != nil {
		return tx.faults.rollbackErr
	}
	return tx.Tx.Rollback()
}
func (tx *transactionFaultTx) Commit() error {
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	if tx.faults.afterCommit != nil {
		tx.faults.afterCommit()
	}
	return tx.faults.commitErr
}

func transactionFaultDatabase(t *testing.T, faults *transactionFaultConnector) (*Database, *sql.DB) {
	t.Helper()
	faults.path = filepath.Join(t.TempDir(), "fault.db")
	db := sql.OpenDB(faults)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`CREATE TABLE connection_probe_parent(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	d := &Database{db: db, path: faults.path}
	t.Cleanup(func() { _ = d.Close() })
	return d, db
}

func TestTransactionRollbackFailureRetainsErrors(t *testing.T) {
	for _, panicCase := range []bool{false, true} {
		t.Run(map[bool]string{false: "business_error", true: "panic"}[panicCase], func(t *testing.T) {
			rollbackErr := errors.New("injected rollback IO failure")
			businessErr := errors.New("injected validation failure")
			faults := &transactionFaultConnector{rollbackErr: rollbackErr}
			d, db := transactionFaultDatabase(t, faults)
			var resultErr error
			func() {
				if panicCase {
					defer func() {
						if recovered := recover(); recovered != businessErr {
							t.Errorf("original panic lost: %v", recovered)
						}
					}()
				}
				resultErr = d.WithAccess(context.Background(), func(a *Access) error {
					return a.WithTx(context.Background(), func(executor Executor) error {
						if err := insertTransactionProbe(context.Background(), executor, 1); err != nil {
							return err
						}
						if panicCase {
							panic(businessErr)
						}
						return businessErr
					})
				})
			}()
			if !panicCase && (!errors.Is(resultErr, businessErr) || !errors.Is(resultErr, rollbackErr)) {
				t.Fatalf("original/rollback error lost: %v", resultErr)
			}
			if !errors.Is(d.pauseErr, rollbackErr) || faults.closes.Load() != 1 || faults.rollbacks.Load() != 1 {
				t.Fatalf("cleanup: pause=%v closes=%d rollbacks=%d", d.pauseErr, faults.closes.Load(), faults.rollbacks.Load())
			}
			if db.Stats().OpenConnections != 0 {
				t.Fatal("failed rollback connection retained")
			}
			assertTransactionAccessBlocked(t, d)
		})
	}
}

func TestTransactionCommitResultIsNotRetried(t *testing.T) {
	commitErr := errors.New("injected error after physical commit")
	faults := &transactionFaultConnector{commitErr: commitErr, closeErr: errors.New("injected close error")}
	d, _ := transactionFaultDatabase(t, faults)
	calls := 0
	err := d.WithAccess(context.Background(), func(a *Access) error {
		return a.WithTx(context.Background(), func(executor Executor) error {
			calls++
			return insertTransactionProbe(context.Background(), executor, 1)
		})
	})
	if !errors.Is(err, commitErr) || calls != 1 || faults.closes.Load() != 1 {
		t.Fatalf("commit outcome: err=%v calls=%d closes=%d", err, calls, faults.closes.Load())
	}
	assertTransactionAccessBlocked(t, d)
	// Inspect with a separate test-only connection, not through the paused app.
	inspector, err := sql.Open("sqlite", faults.path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	assertProbeCount(t, inspector, "connection_probe_parent", 1)
}

func TestTransactionCancelAfterCommitIsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	faults := &transactionFaultConnector{afterCommit: cancel}
	d, db := transactionFaultDatabase(t, faults)
	err := d.WithAccess(ctx, func(a *Access) error {
		return a.WithTx(ctx, func(executor Executor) error {
			return insertTransactionProbe(ctx, executor, 1)
		})
	})
	if err != nil || ctx.Err() == nil {
		t.Fatalf("successful commit changed to failure: result=%v context=%v", err, ctx.Err())
	}
	assertProbeCount(t, db, "connection_probe_parent", 1)
}

func TestTransactionCloseWaitsForCleanup(t *testing.T) {
	d, _, _ := transactionTestDatabase(t)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	workDone := make(chan error, 1)
	go func() {
		workDone <- d.WithAccess(context.Background(), func(a *Access) error {
			return a.WithTx(context.Background(), func(executor Executor) error {
				close(entered)
				<-release
				return insertTransactionProbe(context.Background(), executor, 1)
			})
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("transaction did not start")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- d.Close() }()
	waitForDatabaseState(t, d.closing.Load)
	select {
	case err := <-closeDone:
		t.Fatalf("close did not wait for transaction: %v", err)
	default:
	}
	unblock()
	if err := awaitDatabaseResult(t, workDone); err != nil {
		t.Fatal(err)
	}
	if err := awaitDatabaseResult(t, closeDone); err != nil {
		t.Fatal(err)
	}
	if err := d.WithAccess(context.Background(), func(*Access) error { t.Error("closed callback ran"); return nil }); !errors.Is(err, ErrDatabaseClosed) {
		t.Fatalf("closed access = %v", err)
	}
}

func TestTransactionBeginFailureDoesNotRunWork(t *testing.T) {
	beginErr := errors.New("injected begin failure")
	faults := &transactionFaultConnector{beginErr: beginErr}
	d, db := transactionFaultDatabase(t, faults)
	err := d.WithAccess(context.Background(), func(a *Access) error {
		return a.WithTx(context.Background(), func(Executor) error {
			t.Error("business callback ran after failed begin")
			return nil
		})
	})
	if !errors.Is(err, beginErr) || faults.closes.Load() != 1 || db.Stats().OpenConnections != 0 {
		t.Fatalf("begin cleanup: result=%v, closes=%d, stats=%+v", err, faults.closes.Load(), db.Stats())
	}
	assertTransactionAccessBlocked(t, d)
}

func TestTransactionCancelRetainsRollbackFailure(t *testing.T) {
	rollbackErr := errors.New("injected rollback error during cancellation")
	faults := &transactionFaultConnector{rollbackErr: rollbackErr}
	d, _ := transactionFaultDatabase(t, faults)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := d.WithAccess(ctx, func(a *Access) error {
		return a.WithTx(ctx, func(executor Executor) error {
			if err := insertTransactionProbe(ctx, executor, 1); err != nil {
				return err
			}
			cancel()
			return nil
		})
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, rollbackErr) || faults.rollbacks.Load() != 1 {
		t.Fatalf("cancellation hid rollback failure: err=%v, rollbacks=%d", err, faults.rollbacks.Load())
	}
	assertTransactionAccessBlocked(t, d)
}

func TestTransactionCancelledWaiterDoesNotEnterScope(t *testing.T) {
	d, _, _ := transactionTestDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.WithAccess(context.Background(), func(a *Access) error {
		cancel()
		return a.WithTx(ctx, func(Executor) error {
			t.Error("cancelled transaction callback ran")
			return nil
		})
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled transaction: %v", err)
	}

	// The mutex itself is deliberately not cancellable. Cancellation is checked
	// again on acquisition so queued work cannot start after shutdown/cancel.
	d.mu.Lock()
	done := make(chan error, 1)
	queuedCtx, queuedCancel := context.WithCancel(context.Background())
	defer queuedCancel()
	checkedCtx := &transactionCheckedContext{Context: queuedCtx, checked: make(chan struct{})}
	go func() {
		done <- d.WithAccess(checkedCtx, func(*Access) error {
			t.Error("cancelled queued callback ran")
			return nil
		})
	}()
	<-checkedCtx.checked // First Err check saw nil while the lock was held.
	queuedCancel()
	d.mu.Unlock()
	if err := awaitDatabaseResult(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled queued scope: %v", err)
	}
}

type transactionCheckedContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *transactionCheckedContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestTransactionConcurrentScopesRemainSingleConnection(t *testing.T) {
	d, db, _ := transactionTestDatabase(t)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for id := 1; id <= 12; id++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- d.WithAccess(context.Background(), func(a *Access) error {
				return a.WithTx(context.Background(), func(executor Executor) error {
					return insertTransactionProbe(context.Background(), executor, id)
				})
			})
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertProbeCount(t, db, "connection_probe_parent", 12)
	if stats := db.Stats(); stats.MaxOpenConnections != 1 || stats.OpenConnections != 1 || stats.InUse != 0 {
		t.Fatalf("single connection invariant: %+v", stats)
	}
}
