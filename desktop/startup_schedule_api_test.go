package main

import (
	"encoding/json"
	"testing"
)

func TestHomeReadyBindingDoesNotBorrowDatabase(t *testing.T) {
	// 无数据库的宿主仍能接收就绪信号，等待中的维护任务不得借用连接。
	store := newApp(nil)
	service := &APIService{app: store}
	for range 2 {
		var response struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal([]byte(service.Call("startup/home-ready", "POST", "")), &response); err != nil {
			t.Fatal(err)
		}
		if !response.OK {
			t.Fatal("首页就绪信号未被确认")
		}
	}
	select {
	case <-store.startupSchedule.ready:
	default:
		t.Fatal("未发布首页就绪信号")
	}
}
