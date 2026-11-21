package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exam_notify/internal/infrastructure/transfer"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (s *APIService) transferAPI(path string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.transferMu.Lock()
	defer s.transferMu.Unlock()

	var result any
	var err error
	switch path {
	case "data-transfer/export":
		result, err = s.exportData(r.Context())
	case "data-transfer/preview":
		result, err = s.prepareDataRestore(r.Context())
	case "data-transfer/restore":
		var input struct {
			ID string `json:"id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&input); err == nil {
			err = s.restoreData(r.Context(), input.ID)
		}
		result = map[string]bool{"restored": err == nil}
	case "data-transfer/cancel":
		var input struct {
			ID string `json:"id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&input); err == nil &&
			s.transferCandidate != nil && s.transferCandidate.Preview().ID == input.ID {
			s.closeTransfer()
		}
		result = map[string]bool{"cancelled": err == nil}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *APIService) exportData(ctx context.Context) (string, error) {
	if err := transfer.CheckIdle(ctx, s.app.database); err != nil {
		return "", err
	}
	now := time.Now()
	path, err := s.chooseExportPath(now)
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), transfer.Extension) {
		path += transfer.Extension
	}
	if err := transfer.Export(ctx, s.app.database, path, now); err != nil {
		return "", err
	}
	return path, nil
}

func (s *APIService) chooseExportPath(now time.Time) (string, error) {
	dialog := application.Get().Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{
		Title: "导出 StudyFlow 数据", Directory: s.app.DataDirectory(),
		Filename:             "StudyFlow-" + now.Format("20060102-150405") + transfer.Extension,
		Filters:              []application.FileFilter{{DisplayName: "StudyFlow 备份文件", Pattern: "*" + transfer.Extension}},
		CanCreateDirectories: true,
	})
	return dialog.PromptForSingleSelection()
}

func (s *APIService) prepareDataRestore(ctx context.Context) (*transfer.Preview, error) {
	if err := transfer.CheckIdle(ctx, s.app.database); err != nil {
		return nil, err
	}
	s.closeTransfer()
	path, err := s.chooseRestorePath()
	if err != nil || path == "" {
		return nil, err
	}
	current, currentErr := os.Stat(s.app.database.Path())
	selected, selectedErr := os.Stat(path)
	if currentErr == nil && selectedErr == nil && os.SameFile(current, selected) {
		return nil, errors.New("此文件是本机正在使用的数据库，请选择备份文件")
	}
	candidate, err := transfer.Prepare(ctx, path, time.Now())
	if err != nil {
		return nil, err
	}
	s.transferCandidate = candidate
	preview := candidate.Preview()
	return &preview, nil
}

func (s *APIService) chooseRestorePath() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("选择 StudyFlow 备份文件").
		SetDirectory(s.app.DataDirectory()).
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AddFilter("StudyFlow 备份或旧版数据库", "*.studyflow;*.db").
		PromptForSingleSelection()
}

func (s *APIService) restoreData(ctx context.Context, id string) error {
	if s.transferCandidate == nil || s.transferCandidate.Preview().ID != id {
		return errors.New("导入预览已失效，请重新选择文件")
	}
	defer s.closeTransfer()
	return s.transferCandidate.Restore(ctx, s.app.database, func() {
		// 原库任务尚未释放数据库锁，先停旧任务，避免它们唤醒、响铃或写入新库。
		s.app.restartAfterRestore.Store(true)
		s.app.tasks.mu.Lock()
		s.app.tasks.stopLocked()
		s.app.tasks.mu.Unlock()
	})
}

func (s *APIService) closeTransfer() {
	if s.transferCandidate != nil {
		s.transferCandidate.Close()
		s.transferCandidate = nil
	}
}

// ServiceShutdown 退出时释放尚未确认的隔离副本。
func (s *APIService) ServiceShutdown() error {
	s.transferMu.Lock()
	defer s.transferMu.Unlock()
	s.closeTransfer()
	return nil
}
