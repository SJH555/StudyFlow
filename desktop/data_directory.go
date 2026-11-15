//go:build (linux && !android) || windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"exam_notify/internal/infrastructure/persistence"
)

func (s *APIService) setDataDirectory(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errors.New("请选择一个有效的数据目录")
	}
	installDir, err := dataDirectoryInstallDir()
	if err != nil {
		return err
	}
	result, err := s.app.database.SwitchDirectory(context.Background(), dir, s.locationFile, installDir)
	return directorySwitchResponse(dir, result, err)
}

func directorySwitchResponse(target string, result persistence.DirectorySwitchResult, err error) error {
	if result.Committed {
		if warning := errors.Join(result.Warning, err); warning != nil {
			log.Printf("数据目录已切换到 %q，提交后告警 %v", target, warning)
		}
		return nil
	}
	switch {
	case errors.Is(err, persistence.ErrDatabasePaused):
		return fmt.Errorf("无法确认数据目录切换结果，数据库访问已暂停。请退出并重新打开 StudyFlow，以检查恢复记录。%w", err)
	case errors.Is(err, persistence.ErrDirectorySwitchPending):
		return fmt.Errorf("仍有待处理的数据目录切换记录。请退出并重新打开 StudyFlow 后再试。%w", err)
	default:
		return err
	}
}
