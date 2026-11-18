package main

import (
	"exam_notify/internal/infrastructure/persistence"

	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"exam_notify/internal/modules/goals"
)

func (a *App) goalsAPI(db *persistence.Queries, w http.ResponseWriter, r *http.Request, path string) {
	// Call 持有一次独占借用；业务事务复用该范围，不再次申请生命周期锁。
	// 此处仅组装依赖并转发，Repository 不再重复借用生命周期锁。
	service := goals.NewService(goals.NewSQLiteRepository(db))
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := service.List()
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, items)
		case http.MethodPost:
			var input goals.Input
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				fail(w, err)
				return
			}
			item, err := service.Create(input)
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, item)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) < 1 || len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id < 1 {
		fail(w, strconv.ErrSyntax)
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var completed bool
		switch parts[1] {
		case "accept":
			var input struct {
				Completed bool `json:"completed"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				fail(w, err)
				return
			}
			if err := service.Accept(id, input.Completed); err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
			return
		case "complete":
			completed = true
		case "reopen":
			completed = false
		default:
			http.NotFound(w, r)
			return
		}
		if err := service.SetCompleted(id, completed); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input goals.Input
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			fail(w, err)
			return
		}
		input.ID = id
		item, err := service.Update(input)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, item)
	case http.MethodDelete:
		if err := service.Delete(id); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
