package review

import (
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/schedule"
	"fmt"
	"strings"
	"time"
)

// History 返回最近的复习总结历史（含各项明细）。
func (s *Service) History() ([]DailyReview, error) {
	versioned, err := schedule.HasHistory(s.db)
	if err != nil {
		return nil, err
	}
	status, where := ", 'submitted'", ""
	if versioned {
		status, where = ", status", " WHERE status<>'pending'"
	}
	rows, err := s.db.Query(`SELECT ` + reviewColumns + status + ` FROM daily_reviews` + where + ` ORDER BY day DESC LIMIT 120`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DailyReview{}
	for rows.Next() {
		var review DailyReview
		if err = rows.Scan(&review.Day, &review.Mode, &review.Threshold, &review.PlannedSeconds, &review.ActualSeconds, &review.SubmittedAt, &review.Status); err != nil {
			return nil, err
		}
		out = append(out, review)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Items, err = s.itemsForDay(out[i].Day)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) itemsForDay(day string) ([]ReviewItem, error) {
	items, err := s.db.Query(`SELECT subject_id,subject_name,planned_seconds,actual_seconds,reason
		FROM daily_review_items WHERE day=? ORDER BY id`, day)
	if err != nil {
		return nil, err
	}
	defer items.Close()
	out := []ReviewItem{}
	for items.Next() {
		var item ReviewItem
		if err := items.Scan(&item.SubjectID, &item.SubjectName, &item.PlannedSeconds, &item.ActualSeconds, &item.Reason); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, items.Err()
}

// Submit 提交当天的复习总结及各项未达标理由。
func (s *Service) submit(day string, reasons []ReviewItem) error {
	pending, err := s.pending(time.Now())
	if err != nil {
		return err
	}
	if pending == nil || pending.Day != day {
		return errors.New("请先完成当前待填写的总结")
	}
	if len(reasons) != len(pending.Items) {
		return errors.New("请填写全部未达标项目的理由")
	}
	for i := range pending.Items {
		if pending.Items[i].SubjectID != reasons[i].SubjectID || strings.TrimSpace(reasons[i].Reason) == "" || len([]rune(reasons[i].Reason)) > 1000 {
			return fmt.Errorf("第 %d 项理由需填写，且不超过 1000 字", i+1)
		}
	}
	if pending.Status == reviewPending {
		return s.submitStoredReview(day, reasons, time.Now())
	}
	_, err = s.db.Exec(`INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at) VALUES(?,?,?,?,?,?)`, pending.Day, pending.Mode, pending.Threshold, pending.PlannedSeconds, pending.ActualSeconds, time.Now().Format(time.RFC3339))
	if err != nil {
		return err
	}
	for i, item := range pending.Items {
		_, err = s.db.Exec(`INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason) VALUES(?,?,?,?,?,?)`, pending.Day, item.SubjectID, item.SubjectName, item.PlannedSeconds, item.ActualSeconds, strings.TrimSpace(reasons[i].Reason))
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) Submit(day string, reasons []ReviewItem) error {
	return s.db.WithTx(func(tx *persistence.Queries) error { return New(tx).submit(day, reasons) })
}
