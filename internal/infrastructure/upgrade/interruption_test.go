package upgrade

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"modernc.org/sqlite"
)

const interruptedUpgrade2 = `CREATE TABLE upgrade_interrupt_probe(id INTEGER PRIMARY KEY,value TEXT); INSERT INTO upgrade_interrupt_probe VALUES(1,'committed file 2');`
const interruptedUpgrade3 = `UPDATE upgrade_interrupt_probe SET value='uncommitted file 3'; ALTER TABLE sessions ADD COLUMN interrupted_column TEXT; CREATE TABLE upgrade_interrupt_scratch(data BLOB); INSERT INTO upgrade_interrupt_scratch VALUES(zeroblob(8388608)); SELECT studyflow_upgrade_interrupt_checkpoint();`

// This child runs the real Manager -> Goose -> shared persistence transaction
// chain. Its checkpoint is inside the SQL file, after dirty DML and DDL.
func TestUpgradeInterruptionChild(t *testing.T) {
	root := os.Getenv("STUDYFLOW_UPGRADE_INTERRUPT_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	mode := os.Getenv("STUDYFLOW_UPGRADE_INTERRUPT_MODE")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sqlite.RegisterScalarFunction("studyflow_upgrade_interrupt_checkpoint", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		if mode == "cancel" {
			cancel()
			return int64(1), nil
		}
		if mode == "committed" {
			return int64(1), nil
		}
		if err := os.WriteFile(filepath.Join(root, "ready"), []byte("SQL has executed DML and DDL"), 0600); err != nil {
			return nil, err
		}
		// Killed by the parent; no defer or driver cleanup can run.
		time.Sleep(45 * time.Second)
		return nil, fmt.Errorf("parent did not terminate child")
	}); err != nil {
		t.Fatal(err)
	}
	db, err := persistence.OpenUnprepared(filepath.Join(root, "study.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	d, err := persistence.AdoptDatabase(db, filepath.Join(root, "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m := testManager(t, 3, map[int64]string{2: interruptedUpgrade2, 3: interruptedUpgrade3})
	err = d.WithAccess(ctx, func(a *persistence.Access) error {
		s, e := m.Inspect(ctx, a)
		if e != nil {
			return e
		}
		return m.Apply(ctx, a, s)
	})
	if mode == "cancel" {
		if err == nil {
			t.Fatal("cancelled upgrade succeeded")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ready"), []byte("version 3 committed"), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(45 * time.Second)
	t.Fatal("parent did not terminate committed child")
}

func TestUpgradeInterruptionRecovery(t *testing.T) {
	for _, journal := range []string{"delete", "wal"} {
		for _, mode := range []string{"cancel", "uncommitted", "committed"} {
			t.Run(journal+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "study.db")
				db, err := persistence.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec("PRAGMA journal_mode=" + journal); err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(`INSERT INTO subjects(id,name,created_at) VALUES(1,'preserve subject','2026-11-18T08:00:00+08:00'); INSERT INTO sessions(id,subject_id,start_at,end_at,duration_seconds,source,note) VALUES(1,1,'2026-11-18T08:00:00+08:00','2026-11-18T08:03:00+08:00',180,'manual','preserve record')`); err != nil {
					t.Fatal(err)
				}
				if err = db.Close(); err != nil {
					t.Fatal(err)
				}
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(exe, "-test.run=^TestUpgradeInterruptionChild$", "-test.timeout=60s")
				cmd.Env = append(os.Environ(), "STUDYFLOW_UPGRADE_INTERRUPT_ROOT="+root, "STUDYFLOW_UPGRADE_INTERRUPT_MODE="+mode)
				logFile, err := os.Create(filepath.Join(root, "child.log"))
				if err != nil {
					t.Fatal(err)
				}
				defer logFile.Close()
				cmd.Stdout = logFile
				cmd.Stderr = logFile
				if err = cmd.Start(); err != nil {
					t.Fatal(err)
				}
				waited := false
				defer func() {
					if !waited {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()
				if mode == "cancel" {
					err = cmd.Wait()
					waited = true
					if err != nil {
						b, _ := os.ReadFile(filepath.Join(root, "child.log"))
						t.Fatalf("cancel child: %v %s", err, b)
					}
				} else {
					deadline := time.Now().Add(20 * time.Second)
					for {
						if _, e := os.Stat(filepath.Join(root, "ready")); e == nil {
							break
						}
						if time.Now().After(deadline) {
							b, _ := os.ReadFile(filepath.Join(root, "child.log"))
							t.Fatalf("checkpoint timeout: %s", b)
						}
						time.Sleep(10 * time.Millisecond)
					}
					suffix := "-journal"
					if journal == "wal" {
						suffix = "-wal"
					}
					if mode == "uncommitted" {
						info, e := os.Stat(path + suffix)
						if e != nil || info.Size() == 0 {
							t.Fatalf("missing dirty sidecar: %v", e)
						}
					}
					if err = cmd.Process.Kill(); err != nil {
						t.Fatal(err)
					}
					err = cmd.Wait()
					waited = true
					if err == nil {
						t.Fatal("child was not killed")
					}
				}
				reopened, err := persistence.OpenUnprepared(path, false)
				if err != nil {
					t.Fatal(err)
				}
				d, err := persistence.AdoptDatabase(reopened, path)
				if err != nil {
					t.Fatal(err)
				}
				defer d.Close()
				wantVersion := "2"
				wantColumn := "0"
				wantValue := "committed file 2"
				if mode == "committed" {
					wantVersion = "3"
					wantColumn = "1"
					wantValue = "uncommitted file 3"
				}
				for query, want := range map[string]string{`SELECT max(version_id) FROM goose_db_version`: wantVersion, `SELECT count(*) FROM pragma_table_info('sessions') WHERE name='interrupted_column'`: wantColumn, `SELECT value FROM upgrade_interrupt_probe`: wantValue, `SELECT count(*) FROM sqlite_schema WHERE name='upgrade_interrupt_scratch'`: wantColumn, `SELECT note FROM sessions WHERE id=1`: "preserve record", `SELECT source FROM sessions WHERE id=1`: "manual", `SELECT duration_seconds FROM sessions WHERE id=1`: "180", `PRAGMA integrity_check`: "ok"} {
					if got := scalar(t, d, query); got != want {
						t.Fatalf("%s: got %s want %s", query, got, want)
					}
				}
				// Retry with the same upgrade semantics, omitting only the test checkpoint.
				body := interruptedUpgrade3[:len(interruptedUpgrade3)-len("SELECT studyflow_upgrade_interrupt_checkpoint();")]
				if err := apply(t, d, testManager(t, 3, map[int64]string{2: interruptedUpgrade2, 3: body})); err != nil {
					t.Fatal(err)
				}
				if got := scalar(t, d, `SELECT count(*) FROM goose_db_version WHERE version_id=3`); got != "1" {
					t.Fatal("version was duplicated on resume")
				}
				if got := scalar(t, d, `SELECT note FROM sessions WHERE id=1`); got != "preserve record" {
					t.Fatal("original record changed")
				}
				if err := d.WithRead(func(db *sql.DB, _ string) error {
					rows, e := db.Query(`PRAGMA foreign_key_check`)
					if e != nil {
						return e
					}
					defer rows.Close()
					if rows.Next() {
						return fmt.Errorf("foreign key violation")
					}
					return rows.Err()
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
