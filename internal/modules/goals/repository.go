package goals

import (
	"errors"
	"time"
)

// ErrNotFound 表示目标不存在或已删除。
var ErrNotFound = errors.New("目标不存在")

// Repository 是目标服务需要的数据访问能力。
type Repository interface {
	List() ([]Goal, error)
	Create(Goal) (Goal, error)
	UpdateDetails(Goal) (Goal, error)
	SetAcceptance(id int64, acceptedAt, completedAt *time.Time, updatedAt time.Time) error
	Delete(id int64, updatedAt time.Time) error
}
