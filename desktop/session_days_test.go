package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionDaysBindingReadsLocalDatesWithoutChangingSingleDayContract(t *testing.T) {
	service := newAPITestService(t)
	const day = "2026-11-05"
	if _, err := testExec(service.app, `INSERT INTO subjects(id,name,created_at) VALUES(1,'查询测试','2026-11-05');
		INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source)
		VALUES(1,'2026-11-05T12:00:00','2026-11-05T12:20:00',1200,'manual')`); err != nil {
		t.Fatal(err)
	}
	var single []Session
	if err := json.Unmarshal([]byte(service.Call("sessions?date="+day, "GET", "")), &single); err != nil {
		t.Fatal(err)
	}
	var grouped map[string][]Session
	response := service.Call("sessions/batch?date="+day+"&date=2026-11-06&date="+day, "GET", "")
	if err := json.Unmarshal([]byte(response), &grouped); err != nil {
		t.Fatal(err)
	}
	if len(single) != 1 || len(grouped) != 2 || len(grouped[day]) != 1 || grouped[day][0].ID != single[0].ID {
		t.Fatalf("批量与单日响应不一致：%s", response)
	}
	if grouped["2026-11-06"] == nil || len(grouped["2026-11-06"]) != 0 {
		t.Fatal("无记录日期必须编码为空数组")
	}
	for _, request := range []struct{ path, method string }{
		{"sessions/batch?date=2026-02-30", "GET"},
		{"sessions/batch?date=" + day, "POST"},
	} {
		if result := service.Call(request.path, request.method, "{}"); !strings.Contains(result, `"error"`) {
			t.Fatalf("无效请求未被拒绝：%s", result)
		}
	}
}
