package persistence

import (
	"context"
	"database/sql"
)

// Executor is the SQL surface available inside a business transaction. Helpers
// must use this executor, not borrow another connection or start another tx.
type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

var (
	_ Executor = (*sql.DB)(nil)
	_ Executor = (*sql.Tx)(nil)
)

// Only expose SQL operations, so callbacks cannot accidentally commit/rollback
// the transaction owned by the runner.
type transactionExecutor struct{ Executor }
