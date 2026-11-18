//go:build windows && upgradegui

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"exam_notify/internal/infrastructure/persistence"
)

// Executed in a separate native Windows process. A Linux SQLite reader must
// not inspect an active Windows WAL file through WSL's filesystem bridge.
func verifyUpgradeGUIFixture(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected fixture and version")
	}
	root := args[0]
	version, err := strconv.Atoi(args[1])
	if err != nil {
		return err
	}
	marker, err := os.ReadFile(filepath.Join(root, ".studyflow-upgrade-qa"))
	if err != nil || string(marker) != "temporary upgrade UI fixture\n" {
		return fmt.Errorf("not an owned fixture")
	}
	raw, err := os.ReadFile(filepath.Join(root, "original-business.json"))
	if err != nil {
		return err
	}
	var expected map[string]struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err = json.Unmarshal(raw, &expected); err != nil {
		return err
	}
	checkDB := func(path string, isBackup bool) (int, error) {
		db, err := persistence.OpenUnprepared(path, false)
		if err != nil {
			return 0, err
		}
		defer db.Close()
		var integrity string
		if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
			return 0, fmt.Errorf("integrity=%s: %v", integrity, err)
		}
		fk, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			return 0, err
		}
		bad := fk.Next()
		err = fk.Err()
		fk.Close()
		if bad || err != nil {
			return 0, fmt.Errorf("foreign key check: %v %v", bad, err)
		}
		var actualVersion int
		if err = db.QueryRow("SELECT max(version_id) FROM goose_db_version").Scan(&actualVersion); err != nil {
			return 0, err
		}
		want := version
		if isBackup {
			want = actualVersion
			if want != 1 && want != 2 {
				return 0, fmt.Errorf("backup version %d", want)
			}
		}
		versions, err := db.Query("SELECT version_id,is_applied FROM goose_db_version ORDER BY id")
		if err != nil {
			return 0, err
		}
		count := 0
		for versions.Next() {
			var v, applied int
			if err = versions.Scan(&v, &applied); err != nil {
				versions.Close()
				return 0, err
			}
			if v != count || applied != 1 {
				versions.Close()
				return 0, fmt.Errorf("invalid ledger")
			}
			count++
		}
		err = versions.Err()
		versions.Close()
		if err != nil {
			return 0, err
		}
		if count != want+1 {
			return 0, fmt.Errorf("version count %d, expected %d", count, want+1)
		}
		for table, saved := range expected {
			quoted := make([]string, len(saved.Columns))
			for i, col := range saved.Columns {
				quoted[i] = `"` + strings.ReplaceAll(col, `"`, `""`) + `"`
			}
			rows, err := db.Query(`SELECT ` + strings.Join(quoted, ",") + ` FROM "` + table + `" ORDER BY rowid`)
			if err != nil {
				return 0, err
			}
			actual := make([][]any, 0)
			for rows.Next() {
				row := make([]any, len(saved.Columns))
				ptrs := make([]any, len(row))
				for i := range row {
					ptrs[i] = &row[i]
				}
				if err = rows.Scan(ptrs...); err != nil {
					rows.Close()
					return 0, err
				}
				actual = append(actual, row)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return 0, err
			}
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(saved.Rows)
			if string(a) != string(b) {
				return 0, fmt.Errorf("original table %s changed: %s != %s", table, a, b)
			}
		}
		var column, scratch int
		if err = db.QueryRow("SELECT count(*) FROM pragma_table_info('sessions') WHERE name='qa_upgrade_column'").Scan(&column); err != nil {
			return 0, err
		}
		if err = db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='upgrade_scratch'").Scan(&scratch); err != nil {
			return 0, err
		}
		wanted := 0
		if !isBackup && version == 3 {
			wanted = 1
		}
		if column != wanted || scratch != wanted {
			return 0, fmt.Errorf("partial DDL: column %d, scratch %d expected %d", column, scratch, wanted)
		}
		if !isBackup || actualVersion == 2 {
			var value string
			if err = db.QueryRow("SELECT value FROM upgrade_probe").Scan(&value); err != nil {
				return 0, err
			}
			expectedValue := "file2 committed"
			if !isBackup && version == 3 {
				expectedValue = "file3 committed"
			}
			if value != expectedValue {
				return 0, fmt.Errorf("probe value %s", value)
			}
		}
		return actualVersion, nil
	}
	if _, err = checkDB(filepath.Join(root, "data", "study.db"), false); err != nil {
		return err
	}
	backups, err := filepath.Glob(filepath.Join(root, "data", "backups", "upgrade-*.db"))
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("no retained backup")
	}
	for _, backup := range backups {
		if _, err = checkDB(backup, true); err != nil {
			return fmt.Errorf("backup %s: %w", backup, err)
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"case": filepath.Base(root), "version": version, "allNineTablesUnchanged": true, "integrity": "ok", "foreignKeys": "ok", "retainedBackups": len(backups), "verifier": "Windows native"})
}
