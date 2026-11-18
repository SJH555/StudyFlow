//go:build (linux && !android) || windows

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestUpgradeHandoffValidatorRejectsCandidateBeforePublication(t *testing.T) {
	d, db, path := transactionTestDatabase(t)
	if _, err := db.Exec(`CREATE TABLE version_probe(version INTEGER);INSERT INTO version_probe VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	marker := errors.New("unexpected candidate version")
	checks := 0
	if err := d.SetCandidateValidator(func(ctx context.Context, exec Executor) error {
		checks++
		var version int
		if err := exec.QueryRowContext(ctx, `SELECT version FROM version_probe`).Scan(&version); err != nil {
			return err
		}
		if version != 1 {
			return marker
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	location := filepath.Join(root, "location.txt")
	ops := defaultDirectorySwitchOps()
	open := ops.openCandidate
	ops.openCandidate = func(path string) (*sql.DB, error) {
		copy, err := open(path)
		if err != nil {
			return nil, err
		}
		if _, err := copy.Exec(`UPDATE version_probe SET version=2`); err != nil {
			copy.Close()
			return nil, err
		}
		return copy, nil
	}
	result, err := d.switchDirectory(context.Background(), target, location, filepath.Join(root, "install"), ops)
	if result.Committed || !errors.Is(err, marker) || checks != 2 {
		t.Fatal(result, err, checks)
	}
	if d.Path() != path {
		t.Fatal("rejected candidate became active")
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM version_probe`).Scan(&version); err != nil || version != 1 {
		t.Fatal(version, err)
	}
}
