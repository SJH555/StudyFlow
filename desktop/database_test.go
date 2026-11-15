package main

import (
	"database/sql"
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

func testRoute(store *App, handle func(*sql.DB, http.ResponseWriter, *http.Request), w http.ResponseWriter, r *http.Request) {
	if err := store.database.WithRead(func(db *sql.DB, _ string) error {
		handle(db, w, r)
		return nil
	}); err != nil {
		fail(w, err)
	}
}

func testSessionAction(store *App, w http.ResponseWriter, r *http.Request, id string) {
	testRoute(store, func(db *sql.DB, w http.ResponseWriter, r *http.Request) {
		store.sessionAction(db, w, r, id)
	}, w, r)
}

func (a *App) reviewConfig() (bool, string, int, string, error) {
	var enabled bool
	var mode, start string
	var threshold int
	err := a.database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		enabled, mode, threshold, start, err = review.New(db).ReviewConfig()
		return err
	})
	return enabled, mode, threshold, start, err
}

func (a *App) reviewForDay(day, mode string, threshold int) (*dailyReview, error) {
	var result *dailyReview
	err := a.database.WithRead(func(db *sql.DB, _ string) error {
		var err error
		result, err = review.New(db).ForDay(day, mode, threshold)
		return err
	})
	return result, err
}
