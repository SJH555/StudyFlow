package settings

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"time"
)

// PlansPausedToday 返回今日计划是否已暂停。
func PlansPausedToday(db *persistence.Queries, now time.Time) (bool, error) {
	pausedDate, err := readSetting(db, "pausedPlanDate")
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return pausedDate == now.Format("2006-01-02"), err
}
