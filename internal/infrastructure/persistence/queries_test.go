package persistence

import (
	"context"
	"errors"
	"testing"
)

func TestQueriesScopesCannotEscapeOrBypassTransaction(t *testing.T) {
	d, db, _ := transactionTestDatabase(t)
	var escaped *Queries
	var pending Row
	var txQueries *Queries
	err := d.WithAccess(context.Background(), func(a *Access) error {
		q := a.Queries(context.Background())
		escaped = q
		pending = q.QueryRow(`SELECT 1`)
		if err := q.WithTx(func(tx *Queries) error {
			txQueries = tx
			if _, err := q.Exec(`INSERT INTO connection_probe_parent VALUES(99)`); !errors.Is(err, ErrTransactionNested) {
				t.Fatalf("outer executor bypass: %v", err)
			}
			if err := tx.WithTx(func(*Queries) error { t.Error("nested work ran"); return nil }); !errors.Is(err, ErrTransactionNested) {
				t.Fatalf("nested tx: %v", err)
			}
			return tx.QueryRow(`SELECT 1`).Scan(new(int))
		}); err != nil {
			return err
		}
		if _, err := txQueries.Exec(`INSERT INTO connection_probe_parent VALUES(99)`); !errors.Is(err, ErrAccessExpired) {
			t.Fatalf("expired tx executor: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Exec(`INSERT INTO connection_probe_parent VALUES(99)`); !errors.Is(err, ErrAccessExpired) {
		t.Fatal(err)
	}
	if _, err := escaped.Query(`SELECT 1`); !errors.Is(err, ErrAccessExpired) {
		t.Fatal(err)
	}
	if err := pending.Scan(new(int)); !errors.Is(err, ErrAccessExpired) {
		t.Fatalf("late Scan: %v", err)
	}
	assertProbeCount(t, db, "connection_probe_parent", 0)
}

func TestQueriesContextCancellationPreventsAllSQL(t *testing.T) {
	d, db, _ := transactionTestDatabase(t)
	err := d.WithAccess(context.Background(), func(a *Access) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		q := a.Queries(ctx)
		cancel()
		if _, err := q.Exec(`INSERT INTO connection_probe_parent VALUES(1)`); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if _, err := q.Query(`SELECT 1`); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := q.QueryRow(`SELECT 1`).Scan(new(int)); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := q.WithTx(func(*Queries) error { t.Error("cancelled tx ran"); return nil }); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertProbeCount(t, db, "connection_probe_parent", 0)
	if err := BindQueries(context.Background(), db).WithTx(func(*Queries) error { t.Error("unowned tx ran"); return nil }); !errors.Is(err, ErrTransactionScopeRequired) {
		t.Fatal(err)
	}
}

func TestQueriesBusinessErrorIsPreservedAndRolledBack(t *testing.T) {
	d, db, _ := transactionTestDatabase(t)
	want := errors.New("original business error")
	err := d.WithAccess(context.Background(), func(a *Access) error {
		return a.Queries(context.Background()).WithTx(func(q *Queries) error {
			if _, err := q.Exec(`INSERT INTO connection_probe_parent VALUES(1)`); err != nil {
				return err
			}
			return want
		})
	})
	if err != want {
		t.Fatalf("business error changed: %v", err)
	}
	assertProbeCount(t, db, "connection_probe_parent", 0)
}
