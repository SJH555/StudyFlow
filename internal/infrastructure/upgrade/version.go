// Package upgrade owns database upgrades, not ordinary business persistence.
package upgrade

import (
	"embed"
	"errors"
	"io/fs"
)

// TargetVersion is maintained manually, independently of the APP release.
const TargetVersion int64 = 2
const versionTable = "goose_db_version"

var (
	ErrNewerDatabase  = errors.New("database requires a newer application")
	ErrUnknownSchema  = errors.New("unrecognized database structure")
	ErrInvalidHistory = errors.New("invalid database upgrade history")
)

//go:embed sql/*.sql
var scripts embed.FS

func New() (*Manager, error) {
	source, err := fs.Sub(scripts, "sql")
	if err != nil {
		return nil, err
	}
	manager, err := newManager(source, TargetVersion)
	if err != nil {
		return nil, err
	}
	manager.validateVersion = validateScheduleHistory
	return manager, nil
}
