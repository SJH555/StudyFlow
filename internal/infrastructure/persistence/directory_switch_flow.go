//go:build (linux && !android) || windows

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"exam_notify/internal/infrastructure/storage"
)

type directorySwitchOps struct {
	prepare       func(context.Context, *sql.DB, string, string, string) (*preparedDirectory, error)
	openCandidate func(string) (*sql.DB, error)
	stage         func(*storage.LocationSwitch, string, string, string) error
	publish       func(*storage.LocationSwitch) (storage.LocationResolution, error)
	clear         func(*storage.LocationSwitch) error
	closeDB       func(*sql.DB) error
}

func defaultDirectorySwitchOps() directorySwitchOps {
	return directorySwitchOps{
		prepare: prepareDirectory, openCandidate: OpenExisting, stage: (*storage.LocationSwitch).Stage,
		publish: (*storage.LocationSwitch).Publish, clear: (*storage.LocationSwitch).Clear, closeDB: (*sql.DB).Close,
	}
}

func (d *Database) switchDirectory(ctx context.Context, targetDir, locationFile, installDir string, ops directorySwitchOps) (DirectorySwitchResult, error) {
	var result DirectorySwitchResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	err := d.WithWrite(func(current *sql.DB, path string) error {
		var switchErr error
		result, switchErr = d.switchDirectoryLocked(ctx, current, path, targetDir, locationFile, installDir, ops)
		return switchErr
	})
	return result, err
}

func (d *Database) switchDirectoryLocked(ctx context.Context, current *sql.DB, path, targetDir, locationFile, installDir string, ops directorySwitchOps) (result DirectorySwitchResult, resultErr error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if d.switchCleanupErr != nil {
		return result, errors.Join(ErrDirectorySwitchPending, d.switchCleanupErr)
	}
	location, err := storage.OpenLocationSwitch(locationFile)
	if err != nil {
		return result, fmt.Errorf("open storage location: %w", err)
	}
	defer func() {
		if err := location.Close(); err != nil {
			err = fmt.Errorf("close storage location %s: %w", locationFile, err)
			if result.Committed {
				result.Warning = errors.Join(result.Warning, err)
			} else {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	if location.HasPending() {
		return result, ErrDirectorySwitchPending
	}
	if d.candidateValidator != nil {
		if err := d.candidateValidator(ctx, current); err != nil {
			return result, fmt.Errorf("validate source database version: %w", err)
		}
	}
	paths, err := checkDirectoryPaths(path, targetDir, locationFile)
	if err != nil || paths.sameDirectory {
		return result, err
	}
	if err := storage.CheckSwitchTarget(paths.target, installDir); err != nil {
		return result, err
	}
	sourceDir, err := resolveDirectoryPath(filepath.Dir(path))
	if err != nil {
		return result, err
	}
	old := preparedResource{path: filepath.Join(sourceDir, filepath.Base(path))}
	old.info, err = openedDatabaseInfo(old.path)
	if err != nil {
		return result, err
	}
	if !old.info.Mode().IsRegular() {
		return result, fmt.Errorf("source database is not a regular file: %s", old.path)
	}

	prepared, err := ops.prepare(ctx, current, old.path, paths.target, locationFile)
	if err != nil {
		return result, fmt.Errorf("prepare database directory: %w", err)
	}
	var candidate *sql.DB
	var keepPrepared, clearUnchanged bool
	defer func() {
		if !result.Committed {
			resultErr = errors.Join(resultErr, d.discardSwitchCandidate(location, prepared, candidate, keepPrepared, clearUnchanged, ops))
		}
	}()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if prepared.sameDirectory || prepared.databasePath != filepath.Join(paths.target, "study.db") {
		return result, errors.New("prepared database path does not match the requested directory")
	}
	target, err := switchTargetResource(prepared)
	if err != nil {
		return result, err
	}
	if err := checkSwitchCandidate(target, false); err != nil {
		keepPrepared = true
		return result, err
	}
	candidate, err = ops.openCandidate(target.path)
	if err != nil {
		var closeErr *sqliteCloseError
		keepPrepared = errors.As(err, &closeErr)
		return result, fmt.Errorf("open candidate database %s: %w", target.path, err)
	}
	if d.candidateValidator != nil {
		if err := d.candidateValidator(ctx, candidate); err != nil {
			return result, fmt.Errorf("validate candidate database version: %w", err)
		}
	}
	if err := checkSwitchCandidate(target, true); err != nil {
		keepPrepared = true
		return result, err
	}
	if err := old.checkIdentity(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	location.PlanRetirement(prepared.sourceBackups)
	if err := ops.stage(location, old.path, target.path, installDir); err != nil {
		if location.HasPending() {
			keepPrepared = true
			d.switchCleanupErr = err
		}
		return result, fmt.Errorf("stage storage location: %w", err)
	}
	// 在让暂存记录发布前，再次确认两个文件仍然匹配。
	if err := errors.Join(old.checkIdentity(), checkSwitchCandidate(target, true)); err != nil {
		keepPrepared = true
		d.switchCleanupErr = err
		return result, err
	}
	if err := ctx.Err(); err != nil {
		// Stage leaves an unresolved recovery record. Confirm the old location
		// before clearing it; cancellation alone does not establish disk state.
		resolution, recoverErr := location.Recover()
		if recoverErr == nil && resolution.Outcome == storage.LocationUnchanged && resolution.DatabasePath == old.path {
			clearUnchanged = true
			return result, err
		}
		keepPrepared = true
		d.pauseErr = errors.Join(ErrDatabasePaused, err, recoverErr, errors.New("cannot confirm unchanged storage location after cancellation"))
		return result, d.pauseErr
	}
	// Publication is the commit point. Once confirmed, cancellation cannot undo it.
	resolution, publishErr := ops.publish(location)
	switch {
	case resolution.Outcome == storage.LocationUnchanged && resolution.DatabasePath == old.path:
		clearUnchanged = true
		return result, errors.Join(errors.New("storage location was not published"), publishErr)
	case resolution.Outcome == storage.LocationCommitted && resolution.DatabasePath == target.path:
		d.db, d.path = candidate, target.path
		result.Committed = true
		result.Retired = RetiredDatabase{Path: old.path, Identity: old.info, CloseErr: ops.closeDB(current)}
		result.Warning = publishErr
		if result.Retired.CloseErr != nil {
			result.Warning = errors.Join(result.Warning, fmt.Errorf("close retired database %s: %w", old.path, result.Retired.CloseErr))
		} else if err := old.checkIdentity(); err != nil {
			result.Warning = errors.Join(result.Warning, err)
		} else {
			result.Retired.CleanupAllowed = true
		}
		if !result.Retired.CleanupAllowed {
			d.switchCleanupErr = errors.New("retired database cannot be safely cleaned; restart to retry")
			result.Warning = errors.Join(result.Warning, d.switchCleanupErr)
			return result, nil
		}
		if err := location.CleanupRetired(); err != nil {
			d.switchCleanupErr = err
			result.Warning = errors.Join(result.Warning, err)
			return result, nil
		}
		result.Warning = errors.Join(result.Warning, d.clearSwitchLocation(location, ops))
		return result, nil
	default:
		keepPrepared = true
		if resolution.Outcome != storage.LocationUncertain {
			publishErr = errors.Join(publishErr, fmt.Errorf("unexpected storage location resolution %d for %q", resolution.Outcome, resolution.DatabasePath))
		}
		d.pauseErr = errors.Join(ErrDatabasePaused, publishErr)
		return result, d.pauseErr
	}
}

func (d *Database) clearSwitchLocation(location *storage.LocationSwitch, ops directorySwitchOps) error {
	if err := ops.clear(location); err != nil {
		d.switchCleanupErr = fmt.Errorf("clear pending storage location: %w", err)
		return d.switchCleanupErr
	}
	return nil
}

func (d *Database) discardSwitchCandidate(location *storage.LocationSwitch, prepared *preparedDirectory, candidate *sql.DB, keep, clearUnchanged bool, ops directorySwitchOps) error {
	if candidate != nil {
		if err := ops.closeDB(candidate); err != nil {
			err = fmt.Errorf("close candidate database %s: %w", prepared.databasePath, err)
			if d.pauseErr != nil {
				d.pauseErr = errors.Join(d.pauseErr, err)
			}
			if clearUnchanged || d.switchCleanupErr != nil {
				d.switchCleanupErr = errors.Join(d.switchCleanupErr, err)
			}
			return err
		}
	}
	if clearUnchanged {
		if err := d.clearSwitchLocation(location, ops); err != nil {
			return err
		}
	}
	if keep {
		return nil
	}
	// SQLite 拥有其 sidecar 文件。绝不能在仍有 sidecar 残留时删除主文件，也绝不能作为切换回滚的一部分 unlink 某个 sidecar。
	if err := checkSwitchSidecars(prepared.databasePath, false); err != nil {
		return err
	}
	return prepared.cleanup()
}

func switchTargetResource(prepared *preparedDirectory) (preparedResource, error) {
	for _, resource := range prepared.resources {
		if resource.path == prepared.databasePath && resource.info != nil && resource.info.Mode().IsRegular() {
			return resource, nil
		}
	}
	return preparedResource{}, fmt.Errorf("cannot confirm ownership of prepared database: %s", prepared.databasePath)
}

func checkSwitchCandidate(target preparedResource, allowTransient bool) error {
	if err := target.checkIdentity(); err != nil {
		return err
	}
	return checkSwitchSidecars(target.path, allowTransient)
}

func checkSwitchSidecars(path string, allowTransient bool) error {
	var emptyWAL bool
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		file := path + suffix
		info, err := os.Lstat(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if allowTransient && info.Mode().IsRegular() {
			if suffix == "-wal" && info.Size() == 0 {
				emptyWAL = true
				continue
			}
			if suffix == "-shm" && emptyWAL {
				continue
			}
		}
		return fmt.Errorf("unexpected SQLite sidecar retained: %s", file)
	}
	return nil
}
