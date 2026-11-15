package review

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// History 返回最近的复习总结历史（含各项明细）。
func (s *Service) History() ([]DailyReview, error) {
	rows, err := s.db.Query(`SELECT day,mode,threshold,planned_seconds,actual_seconds,submitted_at FROM daily_reviews ORDER BY day DESC LIMIT 120`)
	if err != nil {
		return nil, err
	}
	out := []DailyReview{}
	for rows.Next() {
		var review DailyReview
		if err = rows.Scan(&review.Day, &review.Mode, &review.Threshold, &review.PlannedSeconds, &review.ActualSeconds, &review.SubmittedAt); err != nil {
			break
		}
		review.Items = []ReviewItem{}
		out = append(out, review)
	}
	if rowErr := rows.Err(); err == nil {
		err = rowErr
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		items, e := s.db.Query(`SELECT subject_id,subject_name,planned_seconds,actual_seconds,reason FROM daily_review_items WHERE day=? ORDER BY id`, out[i].Day)
		if e != nil {
			return nil, e
		}
		for items.Next() {
			var item ReviewItem
			if e = items.Scan(&item.SubjectID, &item.SubjectName, &item.PlannedSeconds, &item.ActualSeconds, &item.Reason); e != nil {
				break
			}
			out[i].Items = append(out[i].Items, item)
		}
		if rowErr := items.Err(); e == nil {
			e = rowErr
		}
		items.Close()
		if e != nil {
			return nil, e
		}
	}

	return out, nil
}

// Submit 提交当天的复习总结及各项未达标理由。
func (s *Service) Submit(day string, reasons []ReviewItem) error {
	pending, err := s.Pending(time.Now())
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at) VALUES(?,?,?,?,?,?)`, pending.Day, pending.Mode, pending.Threshold, pending.PlannedSeconds, pending.ActualSeconds, time.Now().Format(time.RFC3339))
	if err != nil {
		return err
	}
	for i, item := range pending.Items {
		_, err = tx.Exec(`INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason) VALUES(?,?,?,?,?,?)`, pending.Day, item.SubjectID, item.SubjectName, item.PlannedSeconds, item.ActualSeconds, strings.TrimSpace(reasons[i].Reason))
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}
