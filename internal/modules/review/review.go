package review

import (
	"database/sql"
	"encoding/json"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/modules/sessions"
	"exam_notify/internal/modules/settings"
	"sort"
	"strings"
	"time"
)

// Service 提供复习与总结相关的查询和提交能力。
type Service struct{ db *persistence.Queries }

// New 以给定的数据库创建 Service。
func New(db *persistence.Queries) *Service { return &Service{db: db} }

// ReviewItem 表示一个科目在一天内的计划/实际学习时长及未达标原因。
type ReviewItem struct {
	SubjectID      int64  `json:"subjectId"`
	SubjectName    string `json:"subjectName"`
	PlannedSeconds int64  `json:"plannedSeconds"`
	ActualSeconds  int64  `json:"actualSeconds"`
	Reason         string `json:"reason"`
}

// DailyReview 表示某天的复习总结。
type DailyReview struct {
	Day            string       `json:"day"`
	Mode           string       `json:"mode"`
	Threshold      int          `json:"threshold"`
	PlannedSeconds int64        `json:"plannedSeconds"`
	ActualSeconds  int64        `json:"actualSeconds"`
	Status         string       `json:"status"`
	SubmittedAt    string       `json:"submittedAt,omitempty"`
	Items          []ReviewItem `json:"items"`
}

// BulkReviewOffer 表示一批连续未达标日期的批量总结建议。
type BulkReviewOffer struct {
	StartDay         string        `json:"startDay"`
	EndDay           string        `json:"endDay"`
	DaysWithoutStudy int           `json:"daysWithoutStudy"`
	PendingCount     int           `json:"pendingCount"`
	Reviews          []DailyReview `json:"-"`
}

// ReviewConfig 返回复习配置。
func (s *Service) ReviewConfig() (bool, string, int, string, error) {
	return settings.ReviewConfig(s.db)
}

// 最早未完成的日期优先返回。检查小时之前的日期永不判为到期。
func (s *Service) pending(now time.Time) (*DailyReview, error) {
	enabled, mode, threshold, start, err := s.ReviewConfig()
	if err != nil || !enabled {
		return nil, err
	}
	checkHour, err := settings.CheckHour(s.db)
	if err != nil {
		return nil, err
	}
	checkMinute, err := settings.CheckMinute(s.db)
	if err != nil {
		return nil, err
	}
	if now.Hour()*60+now.Minute() < checkHour*60+checkMinute {
		return nil, nil
	}
	first, err := time.ParseInLocation("2006-01-02", start, now.Location())
	if err != nil {
		first = now.AddDate(0, 0, -1)
	}
	last := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	versioned, err := schedule.HasHistory(s.db)
	if err != nil {
		return nil, err
	}
	if versioned {
		if err := s.settle(first, last, mode, threshold); err != nil {
			return nil, err
		}
		// 开启日期只限制新检查的范围，已经生成的待办不能因此丢失。
		var day string
		err := s.db.QueryRow(`SELECT day FROM daily_reviews
			WHERE status='pending' AND day<? ORDER BY day LIMIT 1`, last.Format(time.DateOnly)).Scan(&day)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return s.storedReview(day)
	}
	for day := first; day.Before(last); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		var exists int
		if err = s.db.QueryRow(`SELECT count(*) FROM daily_reviews WHERE day=?`, key).Scan(&exists); err != nil {
			return nil, err
		}
		if exists != 0 {
			continue
		}
		review, err := s.forDay(key, mode, threshold)
		if err != nil {
			return nil, err
		}
		if len(review.Items) != 0 {
			return review, nil
		}
	}
	return nil, nil
}

// ForDay 生成指定日期的复习总结，包含计划与实际时长对比。
func (s *Service) forDay(day, mode string, threshold int) (*DailyReview, error) {
	planned, err := s.planTargetsForDay(day)
	if err != nil {
		return nil, err
	}
	actual, err := sessions.StudySecondsBySubject(s.db, day)
	if err != nil {
		return nil, err
	}
	review := &DailyReview{Day: day, Mode: mode, Threshold: threshold, Items: []ReviewItem{}}
	ids := make([]int64, 0, len(planned))
	for id := range planned {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		item := planned[id]
		item.ActualSeconds = actual[id]
		review.PlannedSeconds += item.PlannedSeconds
		review.ActualSeconds += item.ActualSeconds
		if mode == "project" && item.PlannedSeconds > 0 && item.ActualSeconds*100 < item.PlannedSeconds*int64(threshold) {
			review.Items = append(review.Items, *item)
		}
	}
	for id, seconds := range actual {
		if planned[id] == nil {
			review.ActualSeconds += seconds
		}
	}
	if mode == "total" && review.PlannedSeconds > 0 && review.ActualSeconds*100 < review.PlannedSeconds*int64(threshold) {
		review.Items = append(review.Items, ReviewItem{SubjectID: 0, SubjectName: "全部学习项目", PlannedSeconds: review.PlannedSeconds, ActualSeconds: review.ActualSeconds})
	}
	return review, nil
}

// BulkOffer 针对连续至少四个完整无记录日提供一次统一总结建议。
// 没有学习计划的日子也计入连续天数，但无需生成总结行。
func (s *Service) bulkOffer(now time.Time, pending *DailyReview) (*BulkReviewOffer, error) {
	if pending == nil {
		return nil, nil
	}
	hasRecord, err := sessions.HasStudyRecord(s.db, pending.Day)
	if err != nil || hasRecord {
		return nil, err
	}
	_, mode, threshold, start, err := s.ReviewConfig()
	if err != nil {
		return nil, err
	}
	first, err := time.ParseInLocation("2006-01-02", start, now.Location())
	if err != nil {
		first = now
	}
	day, err := time.ParseInLocation("2006-01-02", pending.Day, now.Location())
	if err != nil {
		return nil, err
	}
	begin := day
	for candidate := day.AddDate(0, 0, -1); !candidate.Before(first); candidate = candidate.AddDate(0, 0, -1) {
		found, checkErr := sessions.HasStudyRecord(s.db, candidate.Format("2006-01-02"))
		if checkErr != nil {
			return nil, checkErr
		}
		if found {
			break
		}
		begin = candidate
	}
	end := day
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for candidate := day.AddDate(0, 0, 1); candidate.Before(today); candidate = candidate.AddDate(0, 0, 1) {
		found, checkErr := sessions.HasStudyRecord(s.db, candidate.Format("2006-01-02"))
		if checkErr != nil {
			return nil, checkErr
		}
		if found {
			break
		}
		end = candidate
	}
	days := 0
	for cursor := begin; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		days++
	}
	if days <= 3 {
		return nil, nil
	}
	offer := &BulkReviewOffer{StartDay: begin.Format("2006-01-02"), EndDay: end.Format("2006-01-02"), DaysWithoutStudy: days, Reviews: []DailyReview{}}
	for cursor := begin; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		key := cursor.Format("2006-01-02")
		if pending.Status == reviewPending {
			item, err := s.storedReview(key)
			if err != nil {
				return nil, err
			}
			if item != nil && item.Status == reviewPending {
				offer.Reviews = append(offer.Reviews, *item)
			}
			continue
		}
		var submitted int
		if err = s.db.QueryRow(`SELECT count(*) FROM daily_reviews WHERE day=?`, key).Scan(&submitted); err != nil {
			return nil, err
		}
		if submitted != 0 {
			continue
		}
		review, reviewErr := s.forDay(key, mode, threshold)
		if reviewErr != nil {
			return nil, reviewErr
		}
		if len(review.Items) != 0 {
			offer.Reviews = append(offer.Reviews, *review)
		}
	}
	offer.PendingCount = len(offer.Reviews)
	if offer.PendingCount == 0 {
		return nil, nil
	}
	return offer, nil
}

// SaveBulk 将一批连续日期按统一原因提交总结。
func (s *Service) saveBulk(now time.Time, startDay, endDay, reason string) (int, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 1000 {
		return 0, errors.New("请填写不超过 1000 字的统一原因")
	}
	pending, err := s.pending(now)
	if err != nil {
		return 0, err
	}
	offer, err := s.bulkOffer(now, pending)
	if err != nil {
		return 0, err
	}
	if offer == nil || offer.StartDay != startDay || offer.EndDay != endDay {
		return 0, errors.New("待总结日期已变化，请重新检查")
	}
	for _, review := range offer.Reviews {
		if review.Status == reviewPending {
			for i := range review.Items {
				review.Items[i].Reason = reason
			}
			if err := s.submitStoredReview(review.Day, review.Items, now); err != nil {
				return 0, err
			}
			continue
		}
		if _, err = s.db.Exec(`INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at) VALUES(?,?,?,?,?,?)`, review.Day, review.Mode, review.Threshold, review.PlannedSeconds, review.ActualSeconds, now.Format(time.RFC3339)); err != nil {
			return 0, err
		}
		for _, item := range review.Items {
			if _, err = s.db.Exec(`INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason) VALUES(?,?,?,?,?,?)`, review.Day, item.SubjectID, item.SubjectName, item.PlannedSeconds, item.ActualSeconds, reason); err != nil {
				return 0, err
			}
		}
	}
	return len(offer.Reviews), nil
}

// 某个日期的计划目标会在之后任何计划或项目编辑前冻结，
// 因此修改今天的日程不会改写昨天的复盘要求。
func (s *Service) planTargetsForDay(day string) (map[int64]*ReviewItem, error) {
	history, err := schedule.HasHistory(s.db)
	if err != nil {
		return nil, err
	}
	if history {
		items, err := plans.DailyPlans(s.db, []string{day}, time.Now())
		if err != nil {
			return nil, err
		}
		result := make(map[int64]*ReviewItem)
		entry := items[day]
		if entry.Kind == "legacy" {
			var saved []ReviewItem
			if err := json.Unmarshal(entry.TargetsJSON, &saved); err != nil {
				return nil, err
			}
			for i := range saved {
				result[saved[i].SubjectID] = &saved[i]
			}
		}
		for _, p := range entry.Plans {
			item := result[p.SubjectID]
			if item == nil {
				item = &ReviewItem{SubjectID: p.SubjectID, SubjectName: p.Subject}
				result[p.SubjectID] = item
			}
			item.PlannedSeconds += int64(max(0, p.Duration-p.BreakMinutes)) * 60
		}
		return result, nil
	}
	var saved string
	err = s.db.QueryRow(`SELECT targets_json FROM daily_plan_targets WHERE day=?`, day).Scan(&saved)
	if err == nil {
		var items []ReviewItem
		if err = json.Unmarshal([]byte(saved), &items); err != nil {
			return nil, err
		}
		out := map[int64]*ReviewItem{}
		for i := range items {
			item := items[i]
			out[item.SubjectID] = &item
		}
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	date, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return nil, err
	}
	targets, err := plans.StudyTargetsForDay(s.db, date)
	if err != nil {
		return nil, err
	}
	planned := make(map[int64]*ReviewItem, len(targets))
	for id, target := range targets {
		planned[id] = &ReviewItem{SubjectID: id, SubjectName: target.SubjectName, PlannedSeconds: target.PlannedSeconds}
	}
	items := make([]ReviewItem, 0, len(planned))
	for _, item := range planned {
		items = append(items, *item)
	}
	data, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`INSERT INTO daily_plan_targets(day,targets_json) VALUES(?,?)`, day, string(data))
	if err != nil {
		return nil, err
	}
	return planned, nil
}

// SnapshotPastDailyTargets 冻结开始日期到昨天之间所有日期的计划目标。
func (s *Service) snapshotPastDailyTargets(now time.Time) error {
	enabled, _, _, start, err := s.ReviewConfig()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	first, err := time.ParseInLocation("2006-01-02", start, now.Location())
	if err != nil {
		first = now.AddDate(0, 0, -1)
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for day := first; day.Before(end); day = day.AddDate(0, 0, 1) {
		if _, err = s.planTargetsForDay(day.Format("2006-01-02")); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SaveBulk(now time.Time, startDay, endDay, reason string) (int, error) {
	var count int
	err := s.db.WithTx(func(tx *persistence.Queries) error {
		var err error
		count, err = New(tx).saveBulk(now, startDay, endDay, reason)
		return err
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) Pending(now time.Time) (*DailyReview, error) {
	var result *DailyReview
	err := s.db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = New(tx).pending(now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ForDay(day, mode string, threshold int) (*DailyReview, error) {
	var result *DailyReview
	err := s.db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = New(tx).forDay(day, mode, threshold)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) BulkOffer(now time.Time, pending *DailyReview) (*BulkReviewOffer, error) {
	var result *BulkReviewOffer
	err := s.db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = New(tx).bulkOffer(now, pending)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) PlanTargetsForDay(day string) (map[int64]*ReviewItem, error) {
	var result map[int64]*ReviewItem
	err := s.db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = New(tx).planTargetsForDay(day)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) SnapshotPastDailyTargets(now time.Time) error {
	return s.db.WithTx(func(tx *persistence.Queries) error { return New(tx).snapshotPastDailyTargets(now) })
}
