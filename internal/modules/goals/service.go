package goals

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Service 负责目标校验和状态变更，持久化通过注入的 Repository 完成。
type Service struct {
	repository Repository
}

// NewService 组装目标服务。
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// List 返回所有未删除的目标。
func (s *Service) List() ([]Item, error) {
	goals, err := s.repository.List()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(goals))
	for _, goal := range goals {
		items = append(items, toItem(goal))
	}
	return items, nil
}

// Create 校验并创建阶段目标。
func (s *Service) Create(input Input) (Item, error) {
	goal, err := validate(input)
	if err != nil {
		return Item{}, err
	}
	now := time.Now()
	goal.CreatedAt, goal.UpdatedAt = now, now
	goal, err = s.repository.Create(goal)
	return toItem(goal), err
}

// Update 修改目标内容，保留原有完成状态和创建时间。
func (s *Service) Update(input Input) (Item, error) {
	if input.ID < 1 {
		return Item{}, ErrNotFound
	}
	goal, err := validate(input)
	if err != nil {
		return Item{}, err
	}
	goal.UpdatedAt = time.Now()
	goal, err = s.repository.UpdateDetails(goal)
	return toItem(goal), err
}

// SetCompleted 保留原有调用语义：完成即验收，取消完成即重新开启。
func (s *Service) SetCompleted(id int64, completed bool) error {
	if !completed {
		return s.Reopen(id)
	}
	return s.Accept(id, true)
}

// Accept 验收目标，并记录是否完成。
func (s *Service) Accept(id int64, completed bool) error {
	if id < 1 {
		return ErrNotFound
	}
	now := time.Now()
	var completedAt *time.Time
	if completed {
		completedAt = &now
	}
	return s.repository.SetAcceptance(id, &now, completedAt, now)
}

// Reopen 将已验收目标重新放回进行中。
func (s *Service) Reopen(id int64) error {
	if id < 1 {
		return ErrNotFound
	}
	return s.repository.SetAcceptance(id, nil, nil, time.Now())
}

// Delete 逻辑删除目标。
func (s *Service) Delete(id int64) error {
	if id < 1 {
		return ErrNotFound
	}
	return s.repository.Delete(id, time.Now())
}

func validate(input Input) (Goal, error) {
	title := strings.TrimSpace(input.Title)
	note := strings.TrimSpace(input.Note)
	if title == "" || utf8.RuneCountInString(title) > 50 {
		return Goal{}, errors.New("目标需为 1–50 个字符")
	}
	day, err := time.Parse(time.DateOnly, input.Deadline)
	if err != nil || day.Format(time.DateOnly) != input.Deadline {
		return Goal{}, errors.New("请输入有效的截止日期")
	}
	if utf8.RuneCountInString(note) > 500 {
		return Goal{}, errors.New("详情不能超过 500 个字符")
	}
	return Goal{ID: input.ID, Title: title, Deadline: input.Deadline, Note: note}, nil
}
