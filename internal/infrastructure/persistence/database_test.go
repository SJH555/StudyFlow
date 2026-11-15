package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func waitForDatabaseState(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for database state")
		}
		runtime.Gosched()
	}
}

func awaitDatabaseResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for database operation")
		return nil
	}
}

func TestDatabaseReplacementWaitsForBorrower(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.db")
	newPath := filepath.Join(dir, "new.db")
	d, err := NewDatabase(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	readDone := make(chan error, 1)
	go func() {
		readDone <- d.WithRead(func(db *sql.DB, _ string) error {
			close(entered)
			<-release
			return db.Ping()
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not enter")
	}

	replaceDone := make(chan error, 1)
	go func() {
		replaceDone <- d.Replace(func(db *sql.DB, path string) (*sql.DB, string, error) {
			next, err := Open(newPath)
			if err != nil {
				return db, path, err
			}
			return next, newPath, db.Close()
		})
	}()
	// 第一个读者仍持有锁。因此无法再次获取读锁说明替换写者正在等待。
	waitForDatabaseState(t, func() bool {
		if d.mu.TryRLock() {
			d.mu.RUnlock()
			return false
		}
		return true
	})
	close(release)
	if err := awaitDatabaseResult(t, readDone); err != nil {
		t.Fatalf("old connection closed during the borrow: %v", err)
	}
	if err := awaitDatabaseResult(t, replaceDone); err != nil {
		t.Fatal(err)
	}
	if err := d.WithRead(func(db *sql.DB, path string) error {
		var file string
		var seq int
		var name string
		if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &file); err != nil {
			return err
		}
		if path != newPath || file != newPath {
			t.Fatalf("replacement path = %q, SQLite file = %q", path, file)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseCloseWaitsAndRejectsNewBorrowers(t *testing.T) {
	d, err := NewDatabase(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	readDone := make(chan error, 1)
	go func() {
		readDone <- d.WithRead(func(db *sql.DB, _ string) error {
			close(entered)
			<-release
			return db.Ping()
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not enter")
	}

	unexpectedUse := func(*sql.DB, string) error { return errors.New("borrow entered during shutdown") }
	queuedWriter := make(chan error, 1)
	go func() { queuedWriter <- d.WithWrite(unexpectedUse) }()
	waitForDatabaseState(t, func() bool {
		if d.mu.TryRLock() {
			d.mu.RUnlock()
			return false
		}
		return true
	})

	closeDone := make(chan error, 1)
	go func() { closeDone <- d.Close() }()
	waitForDatabaseState(t, d.closing.Load)
	for _, borrow := range []func(func(*sql.DB, string) error) error{d.WithRead, d.WithWrite} {
		if err := borrow(unexpectedUse); !errors.Is(err, ErrDatabaseClosed) {
			t.Fatalf("borrow during close = %v", err)
		}
	}
	close(release)
	if err := awaitDatabaseResult(t, readDone); err != nil {
		t.Fatalf("read borrow interrupted by close: %v", err)
	}
	if err := awaitDatabaseResult(t, queuedWriter); !errors.Is(err, ErrDatabaseClosed) {
		t.Fatalf("queued writer after close began = %v", err)
	}
	if err := awaitDatabaseResult(t, closeDone); err != nil {
		t.Fatal(err)
	}
	if err := d.WithRead(unexpectedUse); !errors.Is(err, ErrDatabaseClosed) {
		t.Fatalf("borrow after close = %v", err)
	}
}

type countedSQLiteConnector struct {
	path     string
	closes   *atomic.Int32
	closeErr error
}

func (c countedSQLiteConnector) Driver() driver.Driver { return &sqlite.Driver{} }

func (c countedSQLiteConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.Driver().Open(c.path)
	if err != nil {
		return nil, err
	}
	return countedSQLiteConn{Conn: conn, closes: c.closes, closeErr: c.closeErr}, nil
}

type countedSQLiteConn struct {
	driver.Conn
	closes   *atomic.Int32
	closeErr error
}

func (c countedSQLiteConn) Close() error {
	c.closes.Add(1)
	return errors.Join(c.Conn.Close(), c.closeErr)
}

func TestDatabaseCloseIsIdempotentAndRetainsFirstError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "driver close error", err: errors.New("SQLite close failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "study.db")
			var closes atomic.Int32
			db := sql.OpenDB(countedSQLiteConnector{path: path, closes: &closes, closeErr: tc.err})
			db.SetMaxOpenConns(1)
			d := &Database{db: db, path: path}
			t.Cleanup(func() { _ = d.Close() })
			var value int
			if err := db.QueryRow(`SELECT 42`).Scan(&value); err != nil || value != 42 {
				t.Fatalf("open SQLite connection: value = %d, error = %v", value, err)
			}
			if count := closes.Load(); count != 0 {
				t.Fatalf("driver closed before Database.Close: %d", count)
			}
			first := d.Close()
			if !errors.Is(first, tc.err) || closes.Load() != 1 {
				t.Fatalf("first Close = %v, driver closes = %d", first, closes.Load())
			}
			for range 2 {
				if again := d.Close(); again != first || closes.Load() != 1 {
					t.Fatalf("repeated Close = %v, first = %v, driver closes = %d", again, first, closes.Load())
				}
			}
		})
	}
}

func TestAdoptDatabaseKeepsValidatedConnectionAndSkipsInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study #100%.db")
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = seed.Exec(`DELETE FROM settings WHERE key='dailyReviewEnabled';
		INSERT INTO subjects(name,color,created_at) VALUES('handoff','#7668e8','2026-11-08')`)
	if err := errors.Join(err, seed.Close()); err != nil {
		t.Fatal(err)
	}
	raw, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	closeHandle := raw.Close
	t.Cleanup(func() {
		if closeHandle != nil {
			_ = closeHandle()
		}
	})
	// 该 TEMP 表证明原始物理连接在交接后仍存活。
	if _, err := raw.Exec(`CREATE TEMP TABLE handoff_marker (value TEXT);
		INSERT INTO handoff_marker VALUES ('original connection')`); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	database, err := AdoptDatabase(raw, path)
	if err != nil {
		t.Fatal(err)
	}
	closeHandle = database.Close
	if database.Path() != path {
		t.Fatalf("database path = %q, want %q", database.Path(), path)
	}
	if err := database.WithRead(func(db *sql.DB, borrowedPath string) error {
		if db != raw || borrowedPath != path {
			t.Fatalf("handoff changed handle or path: %p, %q", db, borrowedPath)
		}
		for _, tc := range []struct{ query, want string }{
			{`SELECT value FROM handoff_marker`, "original connection"},
			{`SELECT color FROM subjects WHERE name='handoff'`, "#7668e8"},
			{`SELECT count(*) FROM settings WHERE key='dailyReviewEnabled'`, "0"},
		} {
			var value string
			if err := db.QueryRow(tc.query).Scan(&value); err != nil || value != tc.want {
				t.Fatalf("%s: got %q, %v; want %q", tc.query, value, err, tc.want)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err = closeHandle()
	closeHandle = nil
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Ping(); err == nil {
		t.Fatal("Database.Close did not close the adopted handle")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("handoff changed the main database file: %v", err)
	}
}

func TestAdoptDatabaseRejectsInvalidHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	closed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		db   *sql.DB
	}{
		{"nil", nil},
		{"zero value", &sql.DB{}},
		{"closed", closed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if owner, err := AdoptDatabase(tc.db, path); owner != nil || err == nil {
				t.Fatalf("invalid handoff returned %v, %v", owner, err)
			}
		})
	}

	raw, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	borrowed, err := raw.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer borrowed.Close()
	if owner, err := AdoptDatabase(raw, path); owner != nil || err == nil {
		t.Fatalf("handoff during a borrow returned %v, %v", owner, err)
	}

	memory, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer memory.Close()
	if owner, err := AdoptDatabase(memory, path); owner != nil || err == nil {
		t.Fatalf("in-memory handoff returned %v, %v", owner, err)
	}
	if err := memory.Ping(); err != nil {
		t.Fatalf("rejected handle was closed by handoff: %v", err)
	}
}

func TestAdoptDatabaseFailedHandoffLeavesCloseToCaller(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "study.db")
	var closes atomic.Int32
	raw := sql.OpenDB(countedSQLiteConnector{path: path, closes: &closes})
	t.Cleanup(func() {
		if err := raw.Close(); err != nil || closes.Load() != 1 {
			t.Errorf("caller close = %v, driver closes = %d", err, closes.Load())
		}
	})
	if err := raw.Ping(); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "another.db")
	if err := os.WriteFile(other, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", " \t ", dir, filepath.Join(dir, "missing.db"), other} {
		if owner, err := AdoptDatabase(raw, invalid); owner != nil || err == nil {
			t.Fatalf("handoff for path %q returned %v, %v", invalid, owner, err)
		}
		if err := raw.Ping(); err != nil || closes.Load() != 0 {
			t.Fatalf("failed handoff changed ownership: ping = %v, closes = %d", err, closes.Load())
		}
	}
}
