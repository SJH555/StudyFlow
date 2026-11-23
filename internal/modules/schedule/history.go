package schedule

import (
	"exam_notify/internal/infrastructure/persistence"
)

// HasHistory 区分已升级的库和供旧格式兼容验证使用的版本 1 库。
func HasHistory(db interface {
	QueryRow(string, ...any) persistence.Row
}) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE name='plan_versions' AND type='table')`).Scan(&exists)
	return exists, err
}
