package review

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"
)

const (
	reviewPassed  = "passed"
	reviewPending = "pending"
)

const reviewColumns = `day,mode,threshold,planned_seconds,actual_seconds,submitted_at`

// SettleBeforeRuleChange 用旧标准检查尚无结论的过去日期，与设置修改共用事务。
func (s *Service) SettleBeforeRuleChange(values map[string]string, now time.Time) error {
	versioned, err := schedule.HasHistory(s.db)
	if err != nil || !versioned {
		return err
	}
	enabled, mode, threshold, start, err := s.ReviewConfig()
	if err != nil || !enabled {
		return err
	}
	newMode, modeSpecified := values["dailyReviewMode"]
	newThreshold, thresholdSpecified := values["dailyReviewThreshold"]
	if (!modeSpecified || newMode == mode) && (!thresholdSpecified || newThreshold == strconv.Itoa(threshold)) {
		return nil
	}
	now = now.In(studycalendar.Location)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first, err := time.ParseInLocation(time.DateOnly, start, studycalendar.Location)
	if err != nil {
		first = end.AddDate(0, 0, -1)
	}
	return s.settle(first, end, mode, threshold)
}

func (s *Service) settle(first, end time.Time, mode string, threshold int) error {
	rows, err := s.db.Query(`SELECT day FROM daily_reviews WHERE day>=? AND day<?`,
		first.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return err
	}
	known := make(map[string]bool)
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			rows.Close()
			return err
		}
		known[day] = true
	}
	readErr := rows.Err()
	if err := rows.Close(); err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	for day := first; day.Before(end); day = day.AddDate(0, 0, 1) {
		key := day.Format(time.DateOnly)
		if known[key] {
			continue
		}
		result, err := s.forDay(key, mode, threshold)
		if err != nil {
			return err
		}
		if result.PlannedSeconds <= 0 {
			continue
		}
		status := reviewPassed
		if len(result.Items) != 0 {
			status = reviewPending
		}
		if _, err := s.db.Exec(`INSERT INTO daily_reviews(`+reviewColumns+`,status) VALUES(?,?,?,?,?,'',?)`,
			key, mode, threshold, result.PlannedSeconds, result.ActualSeconds, status); err != nil {
			return err
		}
		for _, item := range result.Items {
			if _, err := s.db.Exec(`INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason)
				VALUES(?,?,?,?,?,'')`, key, item.SubjectID, item.SubjectName, item.PlannedSeconds, item.ActualSeconds); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) storedReview(day string) (*DailyReview, error) {
	var record DailyReview
	err := s.db.QueryRow(`SELECT `+reviewColumns+`,status FROM daily_reviews WHERE day=?`, day).Scan(
		&record.Day, &record.Mode, &record.Threshold, &record.PlannedSeconds, &record.ActualSeconds, &record.SubmittedAt, &record.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.Items, err = s.itemsForDay(day)
	return &record, err
}

func (s *Service) submitStoredReview(day string, reasons []ReviewItem, now time.Time) error {
	for _, item := range reasons {
		if _, err := s.db.Exec(`UPDATE daily_review_items SET reason=? WHERE day=? AND subject_id=?`,
			strings.TrimSpace(item.Reason), day, item.SubjectID); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE daily_reviews SET status='submitted',submitted_at=? WHERE day=? AND status='pending'`,
		now.Format(time.RFC3339), day)
	return err
}
