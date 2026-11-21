package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/infrastructure/system"
	"exam_notify/internal/infrastructure/transfer"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// APIService 是暴露给前端的 Wails 服务，负责将绑定调用转发到应用与业务层。
type APIService struct {
	app               *App
	startup           *startupUpgrade
	locationFile      string
	afterReminderAck  func(time.Duration, func())
	transferMu        sync.Mutex
	transferCandidate *transfer.Candidate
}

// GetAutostartEnabled 返回系统开机自启是否已开启。
func (s *APIService) GetAutostartEnabled() (bool, error) {
	done, err := s.beginWork()
	if err != nil {
		return false, err
	}
	defer done()
	return system.AutostartEnabled()
}

// SetAutostartEnabled 设置系统开机自启状态。
func (s *APIService) SetAutostartEnabled(enabled bool) error {
	done, err := s.beginWork()
	if err != nil {
		return err
	}
	defer done()
	return system.SetAutostart(enabled)
}

func (s *APIService) beginWork() (func(), error) {
	_, done, err := s.beginWorkContext()
	return done, err
}

func (s *APIService) beginWorkContext() (context.Context, func(), error) {
	if !s.startup.isReady() {
		return nil, nil, errors.New("数据库尚未准备完成，请先处理启动提示")
	}
	ctx, done, ok := s.app.tasks.enter()
	if !ok {
		return nil, nil, errDesktopStopping
	}
	return ctx, done, nil
}

// GetDataDirectory 返回当前数据保存目录。
func (s *APIService) GetDataDirectory() string {
	done, err := s.beginWork()
	if err != nil {
		return ""
	}
	defer done()
	return s.app.DataDirectory()
}

// ChooseDataDirectory 弹出目录选择对话框，让用户选择新的数据保存目录。
func (s *APIService) ChooseDataDirectory() (string, error) {
	done, err := s.beginWork()
	if err != nil {
		return "", err
	}
	defer done()
	return application.Get().Dialog.OpenFile().
		SetTitle("选择学习数据保存目录").
		SetDirectory(s.app.DataDirectory()).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

// SaveHTML 弹出保存对话框，将内容写入用户选择的 HTML 文件并返回路径。
func (s *APIService) SaveHTML(filename, content string) (string, error) {
	done, err := s.beginWork()
	if err != nil {
		return "", err
	}
	defer done()
	dialog := application.Get().Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{
		Title:                "导出 HTML 文件",
		Filename:             filename,
		Filters:              []application.FileFilter{{DisplayName: "HTML 文件", Pattern: "*.html"}},
		CanCreateDirectories: true,
	})
	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err = os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// SetDataDirectory 切换应用的数据保存目录。
func (s *APIService) SetDataDirectory(dir string) error {
	ctx, done, err := s.beginWorkContext()
	if err != nil {
		return err
	}
	defer done()
	return s.setDataDirectory(ctx, dir)
}

// Call 以统一入口执行前端发起的 HTTP 风格 API 调用，返回响应体字符串。
func (s *APIService) Call(path, method, body string) string {
	ctx, done, workErr := s.beginWorkContext()
	if workErr != nil {
		b, _ := json.Marshal(map[string]string{"error": workErr.Error()})
		return string(b)
	}
	defer done()
	var payload strings.Reader
	payload.Reset(body)
	req, err := http.NewRequestWithContext(ctx, method, "http://wails.local/api/"+strings.TrimPrefix(path, "/"), &payload)
	if err != nil {
		b, _ := json.Marshal(map[string]any{"error": err.Error()})
		return string(b)
	}
	rr := httptest.NewRecorder()
	req, effects := requestWithDatabaseEffects(req)
	if !s.handleDatabaseFreeAPI(apiRoute(req), rr, req) {
		if err := s.app.database.WithAccess(ctx, func(access *persistence.Access) error {
			s.app.handleScopedAPI(access, rr, req)
			return nil
		}); err != nil {
			fail(rr, err)
		}
	}
	if rr.Code < 200 || rr.Code >= 300 {
		b, _ := json.Marshal(map[string]string{"error": strings.TrimSpace(rr.Body.String())})
		return string(b)
	}
	for _, effect := range *effects {
		effect()
	}
	return rr.Body.String()
}
