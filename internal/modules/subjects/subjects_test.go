package subjects_test

import (
	"exam_notify/internal/testutil"
	"path/filepath"
	"strings"
	"testing"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/subjects"
)

func TestSubjectNameLengthOnCreateAndUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	q := testutil.Borrow(t, db, path)
	created, err := subjects.Create(q, subjects.Subject{Name: strings.Repeat("学", 50)})
	if err != nil {
		t.Fatalf("50 字的学习项目名称应可保存: %v", err)
	}
	if _, err := subjects.Create(q, subjects.Subject{Name: strings.Repeat("学", 51)}); err == nil {
		t.Fatal("超过 50 字的名称不应被新建")
	}
	created.Name = strings.Repeat("学", 51)
	if _, err := subjects.Update(q, created); err == nil {
		t.Fatal("超过 50 字的名称不应被编辑")
	}
}
