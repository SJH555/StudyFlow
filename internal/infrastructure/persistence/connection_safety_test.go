package persistence

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// These probes verify database/sql + modernc behavior, not a production
// transaction wrapper. Every database is disposable and file-backed.
func connectionProbeDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "connection-probe.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, query := range []string{
		`CREATE TABLE connection_probe_parent(id INTEGER PRIMARY KEY)`,
		`CREATE TABLE connection_probe_child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES connection_probe_parent(id))`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return db, path
}

func probeConn(t *testing.T, db *sql.DB) *sql.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func requireSQLiteCode(t *testing.T, err error, code int) {
	t.Helper()
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code()&255 != code {
		t.Fatalf("wanted SQLite base code %d, got %v", code, err)
	}
}

func assertProbeCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	// Table names are constants supplied only by tests.
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func TestConnectionSafetyOrdinaryCompletion(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			db, _ := connectionProbeDB(t)
			conn := probeConn(t, db)
			tx, err := conn.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO connection_probe_parent VALUES(1)`); err != nil {
				t.Fatal(err)
			}
			want := 0
			switch outcome {
			case "commit":
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
					t.Fatalf("rollback after success: %v", err)
				}
				want = 1
			case "rollback":
				_, err := tx.Exec(`INSERT INTO connection_probe_parent VALUES(1)`)
				requireSQLiteCode(t, err, 19) // SQLITE_CONSTRAINT
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			case "panic":
				marker := &struct{}{}
				var rollbackErr error
				func() {
					defer func() {
						if recovered := recover(); recovered != marker {
							t.Errorf("panic changed or swallowed: %v", recovered)
						}
					}()
					defer func() { rollbackErr = tx.Rollback() }()
					panic(marker)
				}()
				if rollbackErr != nil {
					t.Fatal(rollbackErr)
				}
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			assertProbeCount(t, db, "connection_probe_parent", want)
			next, err := db.Begin()
			if err != nil {
				t.Fatalf("next transaction failed: %v", err)
			}
			if err := next.Rollback(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConnectionSafetyCommitFailureDiscard(t *testing.T) {
	for _, failure := range []string{"external_reader_busy", "deferred_foreign_key"} {
		t.Run(failure, func(t *testing.T) {
			db, path := connectionProbeDB(t)
			var readerTx *sql.Tx
			if failure == "external_reader_busy" {
				var journal string
				if err := db.QueryRow(`PRAGMA journal_mode=DELETE`).Scan(&journal); err != nil || journal != "delete" {
					t.Fatalf("set rollback journal: %q, %v", journal, err)
				}
				reader, err := OpenExisting(path)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				readerTx, err = reader.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer readerTx.Rollback()
				var count int
				if err := readerTx.QueryRow(`SELECT count(*) FROM connection_probe_parent`).Scan(&count); err != nil {
					t.Fatal(err)
				}
			}
			conn := probeConn(t, db)
			if _, err := conn.ExecContext(context.Background(), `PRAGMA busy_timeout=0`); err != nil {
				t.Fatal(err)
			}
			tx, err := conn.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "deferred_foreign_key" {
				if _, err := tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(`INSERT INTO connection_probe_child VALUES(1,999)`); err != nil {
					t.Fatal(err)
				}
			} else if _, err := tx.Exec(`INSERT INTO connection_probe_parent VALUES(1)`); err != nil {
				t.Fatal(err)
			}
			commitErr := tx.Commit()
			code := 5 // SQLITE_BUSY
			if failure == "deferred_foreign_key" {
				code = 19
			}
			requireSQLiteCode(t, commitErr, code)
			if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("rollback after failed commit: %v", err)
			}
			// Prove the raw SQLite transaction is still active before discarding.
			if _, err := conn.ExecContext(context.Background(), `BEGIN`); err == nil {
				t.Fatal("expected residual transaction after failed commit")
			} else {
				t.Logf("Commit=%v; residual BEGIN=%v", commitErr, err)
			}
			// sql.Conn pins the connection even after sql.Tx is done. Nobody can
			// borrow it from this one-connection pool before cleanup finishes.
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			other, waitErr := db.Conn(ctx)
			cancel()
			if other != nil {
				_ = other.Close()
				t.Fatal("connection escaped pinning")
			}
			if !errors.Is(waitErr, context.DeadlineExceeded) {
				t.Fatalf("waiting caller: %v", waitErr)
			}
			// Raw returning ErrBadConn is database/sql's documented discard
			// path. This is not Conn.Close (which normally returns to the pool).
			if err := conn.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
				t.Fatalf("discard result: %v", err)
			}
			if stats := db.Stats(); stats.OpenConnections != 0 || stats.InUse != 0 {
				t.Fatalf("discard did not remove physical connection: %+v", stats)
			}
			if _, err := conn.ExecContext(context.Background(), `SELECT 1`); !errors.Is(err, sql.ErrConnDone) {
				t.Fatalf("discarded handle remains usable: %v", err)
			}
			if readerTx != nil {
				if err := readerTx.Rollback(); err != nil {
					t.Fatal(err)
				}
			}
			assertProbeCount(t, db, "connection_probe_parent", 0)
			assertProbeCount(t, db, "connection_probe_child", 0)
			var foreignKeys int
			if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
				t.Fatalf("replacement foreign_keys=%d, err=%v", foreignKeys, err)
			}
			next, err := db.Begin()
			if err != nil {
				t.Fatalf("replacement cannot begin: %v", err)
			}
			if _, err := next.Exec(`INSERT INTO connection_probe_parent VALUES(2)`); err != nil {
				t.Fatal(err)
			}
			if err := next.Commit(); err != nil {
				t.Fatal(err)
			}
			assertProbeCount(t, db, "connection_probe_parent", 1)
			if stats := db.Stats(); stats.MaxOpenConnections != 1 || stats.OpenConnections != 1 {
				t.Fatalf("single connection invariant: %+v", stats)
			}
			t.Log("discard removed the residual transaction; replacement committed with foreign keys enabled")
		})
	}
}

func TestConnectionSafetyCloseDoesNotDiscard(t *testing.T) {
	db, _ := connectionProbeDB(t)
	conn := probeConn(t, db)
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO connection_probe_child VALUES(1,999)`); err != nil {
		t.Fatal(err)
	}
	requireSQLiteCode(t, tx.Commit(), 19)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	borrowed := probeConn(t, db)
	// Arrange cleanup before asserting the deliberately unsafe baseline.
	defer func() {
		_ = borrowed.Raw(func(any) error { return driver.ErrBadConn })
	}()
	if _, err := borrowed.ExecContext(context.Background(), `BEGIN`); err == nil {
		t.Fatal("expected Conn.Close to return the connection with residual transaction")
	} else {
		t.Logf("Conn.Close returned the dirty connection to the pool: %v", err)
	}
}
