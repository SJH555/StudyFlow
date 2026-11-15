package settings

import "testing"

func TestSaveNavigationWidths(t *testing.T) {
	db := newGeneralTestDB(t)
	for key, value := range map[string]string{
		"primaryNavWidth": "180",
		"sectionNavWidth": "120",
	} {
		if err := Save(db, map[string]string{key: value}, Actions{}); err != nil {
			t.Fatalf("保存导航宽度失败: %s=%s: %v", key, value, err)
		}
	}
	values, err := List(db)
	if err != nil {
		t.Fatalf("读取应用设置失败: %v", err)
	}
	if values["primaryNavWidth"] != "180" || values["sectionNavWidth"] != "120" {
		t.Fatalf("导航宽度未写入设置表: %+v", values)
	}
}

func TestRejectInvalidNavigationWidths(t *testing.T) {
	db := newGeneralTestDB(t)
	for key, value := range map[string]string{
		"primaryNavWidth": "71",
		"sectionNavWidth": "241",
	} {
		if err := Save(db, map[string]string{key: value}, Actions{}); err == nil {
			t.Fatalf("非法导航宽度应被拒绝: %s=%s", key, value)
		}
	}
}
