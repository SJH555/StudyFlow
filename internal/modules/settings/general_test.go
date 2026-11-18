package settings

import (
	"exam_notify/internal/testutil"
	"path/filepath"
	"testing"

	"exam_notify/internal/infrastructure/persistence"
)

func newGeneralTestDB(t *testing.T) *persistence.Queries {
	t.Helper()
	path := filepath.Join(t.TempDir(), "study.db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return testutil.Borrow(t, db, path)
}

func TestGeneralDefaults(t *testing.T) {
	db := newGeneralTestDB(t)
	config, err := ReadGeneral(db)
	if err != nil {
		t.Fatalf("读取常规设置失败: %v", err)
	}
	if config.RingtoneVolume != 100 || config.StartupWindow != "show" || config.CloseBehavior != "tray" {
		t.Fatalf("默认常规设置不符: %+v", config)
	}
	hour, err := CheckHour(db)
	if err != nil || hour != 6 {
		t.Fatalf("默认检查小时不符: hour=%d err=%v", hour, err)
	}
	minute, err := CheckMinute(db)
	if err != nil || minute != 0 {
		t.Fatalf("默认检查分钟不符: minute=%d err=%v", minute, err)
	}
}

func TestSaveGeneralValid(t *testing.T) {
	db := newGeneralTestDB(t)
	err := Save(db, map[string]string{
		"ringtoneVolume":         "30",
		"startupWindow":          "tray",
		"closeBehavior":          "quit",
		"dailyReviewCheckHour":   "8",
		"dailyReviewCheckMinute": "30",
	}, Actions{})
	if err != nil {
		t.Fatalf("保存常规设置失败: %v", err)
	}
	config, err := ReadGeneral(db)
	if err != nil {
		t.Fatalf("读取常规设置失败: %v", err)
	}
	if config.RingtoneVolume != 30 || config.StartupWindow != "tray" || config.CloseBehavior != "quit" {
		t.Fatalf("保存后的常规设置不符: %+v", config)
	}
	hour, err := CheckHour(db)
	if err != nil || hour != 8 {
		t.Fatalf("保存后的检查小时不符: hour=%d err=%v", hour, err)
	}
	minute, err := CheckMinute(db)
	if err != nil || minute != 30 {
		t.Fatalf("保存后的检查分钟不符: minute=%d err=%v", minute, err)
	}
}

func TestSaveGeneralInvalid(t *testing.T) {
	db := newGeneralTestDB(t)
	for key, value := range map[string]string{
		"ringtoneVolume":         "101",
		"dailyReviewCheckHour":   "24",
		"dailyReviewCheckMinute": "60",
		"startupWindow":          "minimised",
		"closeBehavior":          "destroy",
	} {
		if err := Save(db, map[string]string{key: value}, Actions{}); err == nil {
			t.Fatalf("非法值应被拒绝: %s=%s", key, value)
		}
	}
}

func TestSaveGeneralTriggersCallback(t *testing.T) {
	db := newGeneralTestDB(t)
	called := false
	err := Save(db, map[string]string{"ringtoneVolume": "40"}, Actions{
		GeneralChanged: func() { called = true },
	})
	if err != nil {
		t.Fatalf("保存常规设置失败: %v", err)
	}
	if !called {
		t.Fatal("常规设置变化应触发 GeneralChanged 回调")
	}
}
