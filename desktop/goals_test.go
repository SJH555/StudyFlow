package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"exam_notify/internal/modules/goals"
)

func TestGoalsBindingKeepsContract(t *testing.T) {
	api := newAPITestService(t)
	var created goals.Item
	raw := api.Call("goals", http.MethodPost, `{"title":"复习","deadline":"2026-12-01","note":"详情"}`)
	if err := json.Unmarshal([]byte(raw), &created); err != nil || created.ID == 0 {
		t.Fatalf("创建契约失败: %s, %v", raw, err)
	}
	path := fmt.Sprintf("goals/%d", created.ID)
	if raw := api.Call(path+"/complete", http.MethodPost, "{}"); raw != "{\"ok\":true}\n" {
		t.Fatalf("完成契约失败: %s", raw)
	}
	raw = api.Call(path, http.MethodPut, `{"title":"复习修改","deadline":"2026-12-02","note":"更新详情"}`)
	var updated goals.Item
	if err := json.Unmarshal([]byte(raw), &updated); err != nil || !updated.Completed || updated.ID != created.ID {
		t.Fatalf("编辑契约失败: %s, %v", raw, err)
	}
	if raw := api.Call(path, http.MethodDelete, ""); raw != "{\"ok\":true}\n" {
		t.Fatalf("删除契约失败: %s", raw)
	}
	if raw := api.Call("goals", http.MethodGet, ""); raw != "[]\n" {
		t.Fatalf("删除后列表错误: %s", raw)
	}
	var failure struct {
		Error string `json:"error"`
	}
	raw = api.Call("goals/not-an-id", http.MethodDelete, "")
	if err := json.Unmarshal([]byte(raw), &failure); err != nil || failure.Error == "" {
		t.Fatalf("无效请求未返回错误: %s, %v", raw, err)
	}
}
