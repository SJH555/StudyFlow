package main

import (
	"context"
	"database/sql"
	"exam_notify/internal/infrastructure/persistence"
	"net/http"

	"exam_notify/internal/modules/review"
)

func testExec(store *App, query string, args ...any) (sql.Result, error) {
	var result sql.Result
	err := store.database.WithWrite(func(db *sql.DB, _ string) error {
		var err error
		result, err = db.Exec(query, args...)
		return err
	})
	return result, err
}

// testRow 只存储查询参数。Scan 在租约内执行并完成。
type testRow struct {
	store *App
	query string
	args  []any
}

func testQueryRow(store *App, query string, args ...any) testRow {
	return testRow{store, query, args}
}

func (row testRow) Scan(dest ...any) error {
	return row.store.database.WithRead(func(db *sql.DB, _ string) error {
		return db.QueryRow(row.query, row.args...).Scan(dest...)
	})
}

func testRoute(store *App, handle func(*persistence.Queries, http.ResponseWriter, *http.Request), w http.ResponseWriter, r *http.Request) {
	r, effects := requestWithDatabaseEffects(r)
	if err := store.database.WithAccess(r.Context(), func(a *persistence.Access) error {
		handle(a.Queries(r.Context()), w, r)
		return nil
	}); err != nil {
		fail(w, err)
		return
	}
	for _, effect := range *effects {
		effect()
	}
}

func testSessionAction(store *App, w http.ResponseWriter, r *http.Request, id string) {
	testRoute(store, func(db *persistence.Queries, w http.ResponseWriter, r *http.Request) {
		store.sessionAction(db, w, r, id)
	}, w, r)
}

func (a *App) reviewConfig() (bool, string, int, string, error) {
	var enabled bool
	var mode, start string
	var threshold int
	err := a.database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		enabled, mode, threshold, start, err = review.New(persistence.BindQueries(context.Background(), db)).ReviewConfig()
		return err
	})
	return enabled, mode, threshold, start, err
}

func (a *App) reviewForDay(day, mode string, threshold int) (*dailyReview, error) {
	var result *dailyReview
	err := a.database.WithAccess(context.Background(), func(access *persistence.Access) error {
		db := access.Queries(context.Background())
		var err error
		result, err = review.New(db).ForDay(day, mode, threshold)
		return err
	})
	return result, err
}
