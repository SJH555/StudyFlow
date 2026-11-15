package plans

import (
	"database/sql"
	"exam_notify/internal/modules/schedule"
	"fmt"
)

func checkConflict(db *sql.DB, plan Plan) error {
	start, err := schedule.StartMinute(plan.StartTime)
	if err != nil {
		return err
	}
	days, err := schedule.ParseWeekdays(plan.Weekdays)
	if err != nil {
		return err
	}
	if plan.Duration < 1 || plan.Duration > 1440 {
		return fmt.Errorf("计划时长必须在 1 到 1440 分钟之间")
	}
	rows, err := conflictRows(db, plan.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var existingStart, existingWeekdays, subject string
		var existingDuration int
		if err := rows.Scan(&existingStart, &existingDuration, &existingWeekdays, &subject); err != nil {
			return err
		}
		oldStart, err := schedule.StartMinute(existingStart)
		if err != nil {
			return err
		}
		oldDays, err := schedule.ParseWeekdays(existingWeekdays)
		if err != nil {
			return err
		}
		if day, overlap := schedule.Overlap(start, plan.Duration, days, oldStart, existingDuration, oldDays); overlap {
			return fmt.Errorf("时间冲突：%s的计划与“%s” %s 开始的计划重叠，请调整时间或重复星期", schedule.WeekdayLabels[day], subject, existingStart)
		}
	}
	return rows.Err()
}
