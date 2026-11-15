package goals_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/goals"
)

func newService(t *testing.T) *goals.Service {
	t.Helper()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return goals.NewService(goals.NewSQLiteRepository(db))
}

func TestLifecyclePreservesCompletionAndSoftDeletion(t *testing.T) {
	service := newService(t)
	created, err := service.Create(goals.Input{
		Title: "  完成复习  ", Deadline: "2026-12-01", Note: "  整理错题  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Title != "完成复习" || created.Note != "整理错题" {
		t.Fatalf("创建目标未规范化: %+v", created)
	}
	if err := service.SetCompleted(created.ID, true); err != nil {
		t.Fatal(err)
	}
	completed, err := service.List()
	if err != nil || len(completed) != 1 || !completed[0].Completed {
		t.Fatalf("完成状态未保存: %+v, %v", completed, err)
	}
	updated, err := service.Update(goals.Input{
		ID: created.ID, Title: "完成第二轮复习", Deadline: "2026-12-08", Note: "补充说明",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Completed || updated.CompletedAt != completed[0].CompletedAt || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("编辑意外改变原始状态: %+v", updated)
	}
	if err := service.SetCompleted(created.ID, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := service.List()
	if err != nil || len(reopened) != 1 || reopened[0].Completed || reopened[0].CompletedAt != "" {
		t.Fatalf("重新开启失败: %+v, %v", reopened, err)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := service.List()
	if err != nil || len(remaining) != 0 {
		t.Fatalf("已删除目标仍可见: %+v, %v", remaining, err)
	}
	if err := service.SetCompleted(created.ID, true); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("已删除目标可被完成: %v", err)
	}
	if _, err := service.Update(goals.Input{ID: created.ID, Title: "恢复", Deadline: "2026-12-09"}); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("已删除目标可被编辑: %v", err)
	}
}

func TestAcceptanceRecordsBothOutcomes(t *testing.T) {
	service := newService(t)
	created, err := service.Create(goals.Input{Title: "阶段复习", Deadline: "2026-12-01"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Accept(created.ID, false); err != nil {
		t.Fatal(err)
	}
	items, err := service.List()
	if err != nil || len(items) != 1 || !items[0].Accepted || items[0].Completed || items[0].AcceptedAt == "" {
		t.Fatalf("未完成验收未保存: %+v, %v", items, err)
	}
	if err := service.Reopen(created.ID); err != nil {
		t.Fatal(err)
	}
	items, err = service.List()
	if err != nil || items[0].Accepted || items[0].Completed {
		t.Fatalf("重新开启未清除验收: %+v, %v", items, err)
	}
	if err := service.Accept(created.ID, true); err != nil {
		t.Fatal(err)
	}
	items, err = service.List()
	if err != nil || !items[0].Accepted || !items[0].Completed {
		t.Fatalf("已完成验收未保存: %+v, %v", items, err)
	}
}

func TestInvalidInputsDoNotChangeStoredGoal(t *testing.T) {
	service := newService(t)
	created, err := service.Create(goals.Input{Title: strings.Repeat("学", 50), Deadline: "2028-02-29"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := []goals.Input{
		{Title: " ", Deadline: "2026-12-01"},
		{Title: strings.Repeat("学", 51), Deadline: "2026-12-01"},
		{Title: "复习", Deadline: "2026-02-29"},
		{Title: "复习", Deadline: "2026-2-01"},
		{Title: "复习", Deadline: "2026-12-01", Note: strings.Repeat("字", 501)},
	}
	for _, input := range invalid {
		input.ID = created.ID
		if _, err := service.Update(input); err == nil {
			t.Fatalf("无效编辑被接受: %+v", input)
		}
		if _, err := service.Create(input); err == nil {
			t.Fatalf("无效新建被接受: %+v", input)
		}
	}
	stored, err := service.List()
	if err != nil || len(stored) != 1 || stored[0].Title != created.Title || stored[0].Deadline != created.Deadline {
		t.Fatalf("无效输入改变了数据: %+v, %v", stored, err)
	}
}
