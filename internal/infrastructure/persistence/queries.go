package persistence

import (
	"context"
	"database/sql"
	"errors"
)

var ErrTransactionScopeRequired = errors.New("transaction requires Database.WithAccess")

// Row permits returning lifecycle errors without issuing a query.
type Row interface{ Scan(...any) error }
type failedRow struct{ err error }

func (r failedRow) Scan(...any) error { return r.err }

type scopedRow struct {
	queries *Queries
	query   string
	args    []any
}

func (r scopedRow) Scan(dest ...any) error {
	if err := r.queries.check(); err != nil {
		return err
	}
	return r.queries.exec.QueryRowContext(r.queries.ctx, r.query, r.args...).Scan(dest...)
}

// Queries binds all SQL to one context and an existing lifecycle borrow.
// It owns no pool and must not escape its scope or be used concurrently.
type Queries struct {
	ctx     context.Context
	exec    Executor
	access  *Access
	inTx    bool
	expired bool
}

func (a *Access) Queries(ctx context.Context) *Queries {
	return &Queries{ctx: ctx, exec: a.db, access: a}
}

// BindQueries is for callers that already own an executor's lifecycle (such as
// backup under WithWrite). It cannot start transactions or acquire another pool.
func BindQueries(ctx context.Context, exec Executor) *Queries {
	return &Queries{ctx: ctx, exec: exec}
}

func (q *Queries) check() error {
	if q.expired {
		return ErrAccessExpired
	}
	if q.access != nil {
		if err := q.access.check(); err != nil {
			return err
		}
		if q.access.inProvider || (q.access.inTx && !q.inTx) {
			return ErrTransactionNested
		}
	}
	return q.ctx.Err()
}

func (q *Queries) Exec(query string, args ...any) (sql.Result, error) {
	if err := q.check(); err != nil {
		return nil, err
	}
	return q.exec.ExecContext(q.ctx, query, args...)
}
func (q *Queries) Query(query string, args ...any) (*sql.Rows, error) {
	if err := q.check(); err != nil {
		return nil, err
	}
	return q.exec.QueryContext(q.ctx, query, args...)
}
func (q *Queries) QueryRow(query string, args ...any) Row {
	if err := q.check(); err != nil {
		return failedRow{err}
	}
	return scopedRow{queries: q, query: query, args: args}
}
func (q *Queries) WithTx(work func(*Queries) error) error {
	if err := q.check(); err != nil {
		return err
	}
	if q.inTx {
		return ErrTransactionNested
	}
	if q.access == nil {
		return ErrTransactionScopeRequired
	}
	return q.access.WithTx(q.ctx, func(exec Executor) error {
		tx := &Queries{ctx: q.ctx, exec: exec, access: q.access, inTx: true}
		defer func() { tx.expired = true }()
		return work(tx)
	})
}
