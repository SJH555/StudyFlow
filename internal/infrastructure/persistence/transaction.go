package persistence

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
)

var (
	ErrAccessExpired     = errors.New("database access scope has expired")
	ErrTransactionNested = errors.New("nested database transactions are not supported")
)

// Access is one exclusive lifecycle borrow. It must not escape WithAccess,
// be used concurrently, or reenter Database. It does not own another pool.
type Access struct {
	owner      *Database
	db         *sql.DB
	active     bool
	inTx       bool
	inProvider bool
}

// WithAccess borrows the database once for an entire use case. Transactions
// started on the scope do not reacquire the lifecycle lock. Like WithWrite,
// waiting for this lock is not interruptible; cancellation is checked before
// waiting and again before entering the callback. Shutdown still drains tasks.
func (d *Database) WithAccess(ctx context.Context, use func(*Access) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return d.WithWrite(func(db *sql.DB, _ string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		access := &Access{owner: d, db: db, active: true}
		defer func() { access.active = false }()
		return use(access)
	})
}

func (a *Access) check() error {
	if !a.active {
		return ErrAccessExpired
	}
	if a.owner.pauseErr != nil {
		return a.owner.pauseErr
	}
	return nil
}

// WithTx never retries business work. SQL must use the supplied Executor and
// caller context. Files, notifications and settings callbacks belong outside
// this callback, after a successful return.
func (a *Access) WithTx(ctx context.Context, work func(Executor) error) (resultErr error) {
	if err := a.check(); err != nil {
		return err
	}
	if a.inTx {
		return ErrTransactionNested
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.inProvider {
		return ErrTransactionNested
	}
	conn, err := a.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire transaction connection: %w", err)
	}
	defer func() {
		if err := releaseTransactionConn(conn); err != nil {
			resultErr = errors.Join(resultErr, err)
			log.Printf("database transaction cleanup: %v", err)
		}
	}()
	return a.withPinnedTx(ctx, conn, work)
}

// withPinnedTx owns the transaction, while its caller owns the connection.
// Both ordinary WithTx and the upgrade provider use precisely this lifecycle.
func (a *Access) withPinnedTx(ctx context.Context, conn *sql.Conn, work func(Executor) error) (resultErr error) {
	if err := a.check(); err != nil {
		return err
	}
	if a.inTx {
		return ErrTransactionNested
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.inTx = true
	defer func() { a.inTx = false }()

	// Own rollback explicitly: database/sql's background Context rollback can
	// hide a driver Rollback error behind ErrTxDone. SQL still uses caller ctx;
	// cancellation is checked before Begin and before Commit. Do not hand off
	// this transaction to another goroutine or perform non-cooperative work.
	tx, err := conn.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		// A driver may fail after partially starting a transaction. Do not put
		// that connection back into service merely because no sql.Tx exists.
		return a.quarantine(conn, fmt.Errorf("begin transaction: %w", err))
	}
	finished := false
	defer func() {
		// This defer also runs on panic/Goexit. It does not recover the panic.
		var cleanupErr error
		if !finished {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				cleanupErr = a.quarantine(conn, fmt.Errorf("rollback transaction: %w", rollbackErr))
			}
		}
		if cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
			// A panic has no returned error channel. Keep cleanup failures visible
			// while allowing the original panic to continue unchanged.
			log.Printf("database transaction cleanup: %v", cleanupErr)
		}
	}()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := work(transactionExecutor{Executor: tx}); err != nil {
		if cancelErr := ctx.Err(); cancelErr != nil {
			return errors.Join(err, cancelErr)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		finished = true // sql.Tx is done; Rollback cannot clean this failure.
		return a.quarantine(conn, fmt.Errorf("commit transaction: %w", err))
	}
	finished = true
	// A later cancellation must not turn a successful commit into a failure.
	return nil
}

func releaseTransactionConn(conn *sql.Conn) error {
	err := conn.Close()
	if err == nil || errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return fmt.Errorf("release transaction connection: %w", err)
}

func (a *Access) quarantine(conn *sql.Conn, cause error) error {
	// Returning ErrBadConn removes the pinned connection before any new borrow.
	// database/sql does not expose the physical driver's Close error here, so
	// conservatively pause the database even if removal reports success. No
	// automatic reconnect/retry is attempted after an uncertain transaction.
	err := conn.Raw(func(any) error { return driver.ErrBadConn })
	if err != nil && !errors.Is(err, driver.ErrBadConn) && !errors.Is(err, sql.ErrConnDone) {
		cause = errors.Join(cause, fmt.Errorf("discard transaction connection: %w", err))
	}
	a.owner.pauseErr = errors.Join(errDatabaseUnavailable, cause)
	return a.owner.pauseErr
}

// PinnedTransactionRunner executes on the sole connection borrowed by a SQL
// provider. It may only be called synchronously inside WithTransactionProvider.
type PinnedTransactionRunner func(context.Context, *sql.Conn, func(Executor) error) error

// WithTransactionProvider keeps a provider within the same exclusive access
// scope. It creates no pool or extra connection. The provider must use run for
// ALL its transactions, and close its borrowed connection on error and panic.
func (a *Access) WithTransactionProvider(ctx context.Context, use func(*sql.DB, PinnedTransactionRunner) error) error {
	if err := a.check(); err != nil {
		return err
	}
	if a.inTx || a.inProvider {
		return ErrTransactionNested
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.inProvider = true
	active := true
	defer func() { active = false; a.inProvider = false }()
	run := func(ctx context.Context, conn *sql.Conn, work func(Executor) error) error {
		if !active {
			return ErrAccessExpired
		}
		return a.withPinnedTx(ctx, conn, work)
	}
	return use(a.db, run)
}
