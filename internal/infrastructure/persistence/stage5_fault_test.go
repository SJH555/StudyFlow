package persistence

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// max_page_count produces a real SQLITE_FULL without exhausting the host disk.
func TestStage5SQLiteFullRollsBackWholeTransaction(t *testing.T) {
	for _, journal := range []string{"delete", "wal"} {
		t.Run(journal, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "full.db")
			d, err := NewDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.WithWrite(func(db *sql.DB, _ string) error {
				var mode string
				if err := db.QueryRow(`PRAGMA journal_mode=` + journal).Scan(&mode); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE TABLE full_probe(id INTEGER PRIMARY KEY, value BLOB); INSERT INTO full_probe VALUES(1,'committed'); PRAGMA cache_size=2`); err != nil {
					return err
				}
				var pages, limit int
				if err := db.QueryRow(`PRAGMA page_count`).Scan(&pages); err != nil {
					return err
				}
				if err := db.QueryRow(fmt.Sprintf(`PRAGMA max_page_count=%d`, pages+8)).Scan(&limit); err != nil {
					return err
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = d.WithAccess(context.Background(), func(a *Access) error {
				return a.Queries(context.Background()).WithTx(func(q *Queries) error {
					calls++
					if _, err := q.Exec(`UPDATE full_probe SET value='must rollback' WHERE id=1`); err != nil {
						return err
					}
					_, err := q.Exec(`INSERT INTO full_probe VALUES(2,zeroblob(1048576))`)
					return err
				})
			})
			requireSQLiteCode(t, err, 13)
			if calls != 1 {
				t.Fatalf("callback reran %d times", calls)
			}
			// SQLite may have auto-rolled back before explicit Rollback. If its driver
			// reports cleanup failure, conservatively pausing the owner is intentional.
			t.Logf("real SQLITE_FULL: paused=%t, error=%v", d.pauseErr != nil, err)
			if d.pauseErr != nil {
				if err := d.WithAccess(context.Background(), func(*Access) error { t.Error("paused callback entered"); return nil }); err == nil {
					t.Fatal("paused database admitted work")
				}
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := OpenExisting(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var value string
			var count int
			if err := db.QueryRow(`SELECT value FROM full_probe WHERE id=1`).Scan(&value); err != nil || value != "committed" {
				t.Fatalf("earlier update remained: %s, %v", value, err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM full_probe`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("partial rows=%d, %v", count, err)
			}
			requireStage5Integrity(t, db)
		})
	}
}

type stage5NativeFault struct {
	code int
	name string
}

func (e *stage5NativeFault) Error() string { return "injected SQLite " + e.name }
func (e *stage5NativeFault) Code() int     { return e.code }

// The source DB is real SQLite. Only the selected SQL statement and selected
// cleanup boundary return controlled errors; this does not simulate hardware.
type stage5Connector struct {
	path                          string
	statementErr, errorOnRollback error
	rollbackCalls                 int
}

func (c *stage5Connector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *stage5Connector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.Driver().Open(c.path)
	if err != nil {
		return nil, err
	}
	return &stage5Conn{Conn: conn, owner: c}, nil
}

type stage5Conn struct {
	driver.Conn
	owner *stage5Connector
}

func (c *stage5Conn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &stage5Stmt{Stmt: stmt, owner: c.owner, fail: strings.Contains(query, "stage5-fault")}, nil
}
func (c *stage5Conn) Begin() (driver.Tx, error) {
	tx, err := c.Conn.Begin()
	if err != nil {
		return nil, err
	}
	return &stage5Tx{Tx: tx, owner: c.owner}, nil
}

type stage5Stmt struct {
	driver.Stmt
	owner *stage5Connector
	fail  bool
}

func (s *stage5Stmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.fail {
		return nil, s.owner.statementErr
	}
	return s.Stmt.Exec(args)
}
func (s *stage5Stmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.fail {
		return nil, s.owner.statementErr
	}
	return s.Stmt.Query(args)
}

type stage5Tx struct {
	driver.Tx
	owner *stage5Connector
}

func (tx *stage5Tx) Rollback() error {
	tx.owner.rollbackCalls++
	if tx.owner.errorOnRollback != nil {
		return tx.owner.errorOnRollback
	}
	return tx.Tx.Rollback()
}

func TestStage5ControlledIOAndMemoryErrors(t *testing.T) {
	for _, code := range []int{10, 7} {
		for _, readback := range []bool{false, true} {
			for _, cleanupFails := range []bool{false, true} {
				t.Run(fmt.Sprintf("code=%d/read=%t/rollback-fails=%t", code, readback, cleanupFails), func(t *testing.T) {
					name := "IOERR"
					if code == 7 {
						name = "NOMEM"
					}
					primary := &stage5NativeFault{code: code, name: name}
					cleanup := errors.New("injected rollback I/O failure")
					path := filepath.Join(t.TempDir(), "injected.db")
					c := &stage5Connector{path: path, statementErr: primary}
					if cleanupFails {
						c.errorOnRollback = cleanup
					}
					db := sql.OpenDB(c)
					db.SetMaxOpenConns(1)
					db.SetMaxIdleConns(1)
					if _, err := db.Exec(`CREATE TABLE fault_probe(id INTEGER PRIMARY KEY,value TEXT);INSERT INTO fault_probe VALUES(1,'committed')`); err != nil {
						t.Fatal(err)
					}
					d := &Database{db: db, path: path}
					defer d.Close()
					calls := 0
					err := d.WithAccess(context.Background(), func(a *Access) error {
						return a.Queries(context.Background()).WithTx(func(q *Queries) error {
							calls++
							if _, err := q.Exec(`UPDATE fault_probe SET value='must rollback' WHERE id=1`); err != nil {
								return err
							}
							if readback {
								var value string
								return q.QueryRow(`SELECT value FROM fault_probe /* stage5-fault */`).Scan(&value)
							}
							_, err := q.Exec(`INSERT INTO fault_probe VALUES(2,'failed') /* stage5-fault */`)
							return err
						})
					})
					if !errors.Is(err, primary) || calls != 1 || c.rollbackCalls != 1 {
						t.Fatalf("error/callback/rollback=%v/%d/%d", err, calls, c.rollbackCalls)
					}
					var native *stage5NativeFault
					if !errors.As(err, &native) || native.Code() != code {
						t.Fatalf("original code lost: %v", err)
					}
					if cleanupFails {
						if !errors.Is(err, cleanup) || d.pauseErr == nil || db.Stats().OpenConnections != 0 {
							t.Fatalf("failed cleanup not isolated: %v, %+v", err, db.Stats())
						}
						if err := d.WithAccess(context.Background(), func(*Access) error { t.Error("paused callback entered"); return nil }); err == nil {
							t.Fatal("paused access allowed")
						}
					} else {
						if d.pauseErr != nil {
							t.Fatal(d.pauseErr)
						}
						if err := d.WithAccess(context.Background(), func(a *Access) error {
							return a.Queries(context.Background()).WithTx(func(q *Queries) error {
								_, err := q.Exec(`INSERT INTO fault_probe VALUES(3,'later successful transaction')`)
								return err
							})
						}); err != nil {
							t.Fatal(err)
						}
					}
					if err := d.Close(); err != nil {
						t.Fatal(err)
					}
					reopened, err := sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					defer reopened.Close()
					var value string
					var count int
					if err := reopened.QueryRow(`SELECT value FROM fault_probe WHERE id=1`).Scan(&value); err != nil || value != "committed" {
						t.Fatalf("earlier update remained: %s, %v", value, err)
					}
					if err := reopened.QueryRow(`SELECT count(*) FROM fault_probe WHERE id=2`).Scan(&count); err != nil || count != 0 {
						t.Fatalf("failed row remained: %d, %v", count, err)
					}
					requireStage5Integrity(t, reopened)
				})
			}
		}
	}
}

func requireStage5Integrity(t *testing.T, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%s, %v", integrity, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation after recovery")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// This helper only runs in a child process launched by the parent test.
func TestStage5CrashChild(t *testing.T) {
	root := os.Getenv("STUDYFLOW_STAGE5_CRASH_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	path := filepath.Join(root, "study.db")
	d, err := NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	// Intentionally no Close/rollback in the success path: the parent kills us.
	mode := os.Getenv("STUDYFLOW_STAGE5_CRASH_MODE")
	ready := func() {
		if err := os.WriteFile(filepath.Join(root, "ready"), []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if err := d.WithWrite(func(db *sql.DB, _ string) error {
		var journal string
		if err := db.QueryRow(`PRAGMA journal_mode=` + os.Getenv("STUDYFLOW_STAGE5_JOURNAL")).Scan(&journal); err != nil {
			return err
		}
		_, err := db.Exec(`PRAGMA synchronous=FULL; PRAGMA cache_size=2; PRAGMA cache_spill=ON; PRAGMA wal_autocheckpoint=0`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = d.WithAccess(context.Background(), func(a *Access) error {
		return a.Queries(context.Background()).WithTx(func(q *Queries) error {
			if mode == "migration" {
				if _, err := q.Exec(`ALTER TABLE crash_probe ADD COLUMN new_field TEXT; CREATE TABLE crash_migration_new(id INTEGER)`); err != nil {
					return err
				}
			}
			if _, err := q.Exec(`UPDATE crash_probe SET value='uncommitted' WHERE id=1`); err != nil {
				return err
			}
			for id := 2; id < 18; id++ {
				if _, err := q.Exec(`INSERT INTO crash_probe(id,value) VALUES(?,zeroblob(32768))`, id); err != nil {
					return err
				}
			}
			if mode != "committed" {
				ready()
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	ready()
}

func TestStage5ForcedTerminationRecovery(t *testing.T) {
	for _, journal := range []string{"delete", "wal"} {
		for _, mode := range []string{"uncommitted", "committed", "migration"} {
			t.Run(journal+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "study.db")
				db, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`CREATE TABLE crash_probe(id INTEGER PRIMARY KEY,value BLOB); INSERT INTO crash_probe VALUES(1,'committed baseline')`); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(executable, "-test.run=^TestStage5CrashChild$", "-test.timeout=60s")
				cmd.Env = append(os.Environ(), "STUDYFLOW_STAGE5_CRASH_ROOT="+root, "STUDYFLOW_STAGE5_CRASH_MODE="+mode, "STUDYFLOW_STAGE5_JOURNAL="+journal)
				output, err := os.Create(filepath.Join(root, "child-output.txt"))
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				cmd.Stdout = output
				cmd.Stderr = output
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				waited := false
				defer func() {
					if !waited {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()
				deadline := time.Now().Add(15 * time.Second)
				for {
					if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						data, _ := os.ReadFile(filepath.Join(root, "child-output.txt"))
						t.Fatalf("child did not become ready: %s", data)
					}
					time.Sleep(10 * time.Millisecond)
				}
				// Record sidecar presence to prove we interrupted a real dirty database.
				suffix := "-journal"
				if journal == "wal" {
					suffix = "-wal"
				}
				info, err := os.Stat(path + suffix)
				if mode != "committed" && (err != nil || info.Size() == 0) {
					t.Fatalf("no dirty %s before kill: %v", suffix, err)
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				waitErr := cmd.Wait()
				waited = true
				if waitErr == nil {
					t.Fatal("child exited normally instead of being killed")
				}
				recovered, err := OpenForStartup(path)
				if err != nil {
					t.Fatal(err)
				}
				defer recovered.Close()
				var value string
				var count, columns, tables int
				if err := recovered.QueryRow(`SELECT value FROM crash_probe WHERE id=1`).Scan(&value); err != nil {
					t.Fatal(err)
				}
				if err := recovered.QueryRow(`SELECT count(*) FROM crash_probe`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				want, wantCount := "committed baseline", 1
				if mode == "committed" {
					want = "uncommitted"
					wantCount = 17
				}
				if value != want || count != wantCount {
					t.Fatalf("recovered value/count=%s/%d, want=%s/%d", value, count, want, wantCount)
				}
				if err := recovered.QueryRow(`SELECT count(*) FROM pragma_table_info('crash_probe') WHERE name='new_field'`).Scan(&columns); err != nil {
					t.Fatal(err)
				}
				if err := recovered.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='crash_migration_new'`).Scan(&tables); err != nil {
					t.Fatal(err)
				}
				if columns != 0 || tables != 0 {
					t.Fatalf("uncommitted DDL remained: columns=%d tables=%d", columns, tables)
				}
				requireStage5Integrity(t, recovered)
			})
		}
	}
}

func TestStage5CommitFailureNeverRetriesOrClaimsRollback(t *testing.T) {
	for _, code := range []int{10, 7} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			primary := &stage5NativeFault{code: code, name: fmt.Sprint(code)}
			faults := &transactionFaultConnector{commitErr: primary}
			d, _ := transactionFaultDatabase(t, faults)
			calls := 0
			err := d.WithAccess(context.Background(), func(a *Access) error {
				return a.Queries(context.Background()).WithTx(func(q *Queries) error {
					calls++
					_, err := q.Exec(`INSERT INTO connection_probe_parent VALUES(1)`)
					return err
				})
			})
			if !errors.Is(err, primary) || calls != 1 || d.pauseErr == nil {
				t.Fatalf("uncertain commit=%v calls=%d pause=%v", err, calls, d.pauseErr)
			}
			if err := d.WithAccess(context.Background(), func(*Access) error { t.Error("uncertain commit admitted work"); return nil }); err == nil {
				t.Fatal("uncertain commit did not stop access")
			}
			db, err := sql.Open("sqlite", faults.path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			assertProbeCount(t, db, "connection_probe_parent", 1)
			requireStage5Integrity(t, db)
		})
	}
}

// SQLITE_LIMIT_VDBE_OP makes SQLite itself return SQLITE_NOMEM while preparing
// a statement that needs too many instructions, without exhausting host memory.
func TestStage5SQLiteResourceLimitReturnsNoMem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	d, err := NewDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.db.Exec(`CREATE TABLE memory_probe(id INTEGER PRIMARY KEY,value TEXT);INSERT INTO memory_probe VALUES(1,'committed baseline')`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conn, err := d.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Limit applies to this temporary database's sole connection only.
	_, limitErr := sqlite.Limit(conn, 5, 100) // SQLITE_LIMIT_VDBE_OP
	if err := errors.Join(limitErr, conn.Close()); err != nil {
		t.Fatal(err)
	}
	statements := make([]string, 60)
	for i := range statements {
		statements[i] = fmt.Sprintf("SELECT %d", i)
	}
	calls := 0
	err = d.WithAccess(ctx, func(a *Access) error {
		return a.Queries(ctx).WithTx(func(q *Queries) error {
			calls++
			if _, err := q.Exec(`UPDATE memory_probe SET value='must rollback' WHERE id=1`); err != nil {
				return err
			}
			var result int
			return q.QueryRow(strings.Join(statements, " UNION ALL ")).Scan(&result)
		})
	})
	requireSQLiteCode(t, err, 7)
	t.Logf("real SQLite SQLITE_NOMEM from instruction limit: calls=%d, paused=%t, error=%v", calls, d.pauseErr != nil, err)
	if calls != 1 {
		t.Fatal("business callback retried")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var value string
	if err := reopened.QueryRow(`SELECT value FROM memory_probe WHERE id=1`).Scan(&value); err != nil || value != "committed baseline" {
		t.Fatalf("earlier write remained: %s, %v", value, err)
	}
	requireStage5Integrity(t, reopened)
}
