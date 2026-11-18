package goals

import (
	"database/sql"
	"exam_notify/internal/infrastructure/persistence"
	"fmt"
	"time"
)

// SQLiteRepository 在调用方借用数据库句柄的有效期内访问目标表。
// 它不创建或关闭连接；生命周期锁由桌面 API 统一入口持有。
type SQLiteRepository struct {
	db *persistence.Queries
}

// NewSQLiteRepository 接收应用组装层提供的数据库句柄。
func NewSQLiteRepository(db *persistence.Queries) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

const goalColumns = `id,title,deadline,note,completed_at,accepted_at,created_at,updated_at`

type rowScanner interface {
	Scan(...any) error
}

func scanGoal(row rowScanner) (Goal, error) {
	var goal Goal
	var completedAt sql.NullString
	var acceptedAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(
		&goal.ID, &goal.Title, &goal.Deadline, &goal.Note,
		&completedAt, &acceptedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return Goal{}, err
	}
	goal.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Goal{}, fmt.Errorf("读取目标创建时间: %w", err)
	}
	goal.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return Goal{}, fmt.Errorf("读取目标更新时间: %w", err)
	}
	if completedAt.Valid {
		parsed, err := time.Parse(time.RFC3339, completedAt.String)
		if err != nil {
			return Goal{}, fmt.Errorf("读取目标完成时间: %w", err)
		}
		goal.CompletedAt = &parsed
	}
	if acceptedAt.Valid {
		parsed, err := time.Parse(time.RFC3339, acceptedAt.String)
		if err != nil {
			return Goal{}, fmt.Errorf("读取目标验收时间: %w", err)
		}
		goal.AcceptedAt = &parsed
	}
	return goal, nil
}

// List 查询未删除目标，进行中目标优先。
func (r *SQLiteRepository) List() ([]Goal, error) {
	rows, err := r.db.Query(`SELECT ` + goalColumns + ` FROM stage_goals
		WHERE deleted=0 ORDER BY accepted_at IS NOT NULL,deadline,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Goal{}
	for rows.Next() {
		goal, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, goal)
	}
	return items, rows.Err()
}

// Create 保存新目标并回填标识。
func (r *SQLiteRepository) create(goal Goal) (Goal, error) {
	result, err := r.db.Exec(`INSERT INTO stage_goals
		(title,deadline,note,created_at,updated_at) VALUES(?,?,?,?,?)`,
		goal.Title, goal.Deadline, goal.Note,
		goal.CreatedAt.Format(time.RFC3339), goal.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return Goal{}, err
	}
	goal.ID, err = result.LastInsertId()
	return goal, err
}

// UpdateDetails 原子更新内容并返回同一次事务中的完整状态。
func (r *SQLiteRepository) UpdateDetails(goal Goal) (Goal, error) {
	// 更新和回读处于同一事务，防止完成、删除等并发操作改变返回状态。
	var updated Goal
	err := r.db.WithTx(func(tx *persistence.Queries) error {
		result, err := tx.Exec(`UPDATE stage_goals SET title=?,deadline=?,note=?,updated_at=?
 WHERE id=? AND deleted=0`, goal.Title, goal.Deadline, goal.Note, goal.UpdatedAt.Format(time.RFC3339), goal.ID)
		if err := requireAffected(result, err); err != nil {
			return err
		}
		updated, err = scanGoal(tx.QueryRow(`SELECT `+goalColumns+` FROM stage_goals WHERE id=?`, goal.ID))
		return err
	})
	if err != nil {
		return Goal{}, err
	}
	return updated, nil
}

// SetAcceptance 同时保存验收与完成结果。
func (r *SQLiteRepository) SetAcceptance(id int64, acceptedAt, completedAt *time.Time, updatedAt time.Time) error {
	var acceptedValue, completedValue any
	if acceptedAt != nil {
		acceptedValue = acceptedAt.Format(time.RFC3339)
	}
	if completedAt != nil {
		completedValue = completedAt.Format(time.RFC3339)
	}
	result, err := r.db.Exec(`UPDATE stage_goals SET accepted_at=?,completed_at=?,updated_at=?
		WHERE id=? AND deleted=0`, acceptedValue, completedValue, updatedAt.Format(time.RFC3339), id)
	return requireAffected(result, err)
}

// Delete 仅标记删除，保留记录供数据管理功能使用。
func (r *SQLiteRepository) Delete(id int64, updatedAt time.Time) error {
	result, err := r.db.Exec(`UPDATE stage_goals SET deleted=1,updated_at=? WHERE id=? AND deleted=0`,
		updatedAt.Format(time.RFC3339), id)
	return requireAffected(result, err)
}

func requireAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) Create(goal Goal) (Goal, error) {
	var result Goal
	err := r.db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = NewSQLiteRepository(tx).create(goal)
		return err
	})
	if err != nil {
		return Goal{}, err
	}
	return result, nil
}
