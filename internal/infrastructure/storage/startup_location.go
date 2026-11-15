//go:build (linux && !android) || windows

package storage

import (
	"errors"
	"fmt"
)

// RecoverStartupLocation 仅在存在待处理名称时运行现有恢复协议；调用方须在调用 clear 前校验所选数据库。
func RecoverStartupLocation(locationFile string, use func(path string, unchanged bool, clear func() error) error) (handled bool, resultErr error) {
	pending, err := hasStartupPending(locationFile, pendingLocationName, locationCleanupSuffix)
	if err != nil || !pending {
		return pending, err
	}
	location, err := OpenLocationSwitch(locationFile)
	if err != nil {
		return true, fmt.Errorf("open pending storage location: %w", err)
	}
	defer func() {
		if err := location.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close pending storage location: %w", err))
		}
	}()
	result, err := location.Recover()
	if err != nil {
		return true, fmt.Errorf("recover pending storage location: %w", err)
	}
	switch result.Outcome {
	case LocationCommitted, LocationUnchanged:
		return true, use(result.DatabasePath, result.Outcome == LocationUnchanged, func() error {
			if result.Outcome == LocationCommitted {
				if err := location.CleanupRetired(); err != nil {
					return err
				}
			}
			return location.Clear()
		})
	default:
		return true, errors.New("pending storage location could not be confirmed")
	}
}
