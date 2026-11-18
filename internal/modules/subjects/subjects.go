package subjects

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/sessions"
	"strings"
	"time"
	"unicode/utf8"
)

// Subject 表示一个学习科目。
type Subject struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder *int64 `json:"sortOrder"`
	Archived  bool   `json:"archived"`
	Deleted   bool   `json:"deleted"`
	HasData   bool   `json:"hasData"`
}

// List 返回学习科目列表，可按需包含归档或已删除项。
func List(db *persistence.Queries, includeArchived, includeDeleted bool) ([]Subject, error) {
	query := `SELECT id,name,color,sort_order,archived,deleted,EXISTS(SELECT 1 FROM plans WHERE subject_id=subjects.id AND deleted=0) OR EXISTS(SELECT 1 FROM sessions WHERE subject_id=subjects.id AND deleted=0) FROM subjects`
	conditions := []string{}
	if !includeArchived {
		conditions = append(conditions, `archived=0`)
	}
	if !includeDeleted {
		conditions = append(conditions, `deleted=0`)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY deleted,archived,CASE WHEN sort_order IS NULL THEN 1 ELSE 0 END,sort_order,created_at,id`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subject{}
	for rows.Next() {
		var x Subject
		var archived, deleted, hasData int
		var sortOrder sql.NullInt64
		if err := rows.Scan(&x.ID, &x.Name, &x.Color, &sortOrder, &archived, &deleted, &hasData); err != nil {
			return nil, err
		}
		if sortOrder.Valid {
			x.SortOrder = &sortOrder.Int64
		}
		x.Archived = archived != 0
		x.Deleted = deleted != 0
		x.HasData = hasData != 0
		out = append(out, x)

	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Create 新增一个学习科目。
func create(db *persistence.Queries, x Subject) (Subject, error) {
	x.Name = strings.TrimSpace(x.Name)
	if x.Name == "" {
		return x, errors.New("name is required")
	}
	if utf8.RuneCountInString(x.Name) > 50 {
		return x, errors.New("学习项目名称不能超过 50 字")
	}
	if x.Color == "" {
		x.Color = "#007AFF"
	}
	if x.SortOrder != nil && *x.SortOrder < 1 {
		return x, errors.New("排序号需为正整数")
	}
	res, err := db.Exec(`INSERT INTO subjects(name,color,created_at,sort_order) VALUES(?,?,?,?)`, x.Name, x.Color, time.Now().Format(time.RFC3339), x.SortOrder)
	if err != nil {
		return x, err
	}
	x.ID, err = res.LastInsertId()
	if err != nil {
		return x, err
	}

	return x, nil
}

// Update 更新一个学习科目。
func Update(db *persistence.Queries, x Subject) (Subject, error) {
	var result Subject
	var err error
	err = db.WithTx(func(tx *persistence.Queries) error {
		result, err = update(tx, x)
		return err
	})
	if err != nil {
		return x, err
	}
	return result, nil
}

func update(db *persistence.Queries, x Subject) (Subject, error) {
	x.Name = strings.TrimSpace(x.Name)
	if x.ID == 0 || x.Name == "" {
		return x, errors.New("id and name are required")
	}
	if utf8.RuneCountInString(x.Name) > 50 {
		return x, errors.New("学习项目名称不能超过 50 字")
	}
	if x.Color == "" {
		x.Color = "#007AFF"
	}
	if x.SortOrder != nil && *x.SortOrder < 1 {
		return x, errors.New("排序号需为正整数")
	}
	archived, deleted := 0, 0
	if x.Archived {
		archived = 1
	}
	if x.Deleted {
		linked, err := hasLinkedData(db, x.ID)
		if err != nil {
			return x, err
		}
		if linked {
			return x, errors.New("学习项目已有相关数据，无法删除")
		}
		deleted = 1
	}
	res, err := db.Exec(`UPDATE subjects SET name=?,color=?,sort_order=?,archived=?,deleted=? WHERE id=?`, x.Name, x.Color, x.SortOrder, archived, deleted, x.ID)
	if err != nil {
		return x, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return x, err
	}
	if n == 0 {
		return x, errors.New("learning content not found")
	}

	if err := plans.SetSubjectActive(db, x.ID, !x.Archived && !x.Deleted); err != nil {
		return x, err
	}
	return x, nil
}

// Delete 软删除指定科目。
func Delete(db *persistence.Queries, id string) error {
	return db.WithTx(func(tx *persistence.Queries) error { return deleteSubject(tx, id) })
}

func deleteSubject(db *persistence.Queries, id string) error {
	linked, err := hasLinkedData(db, id)
	if err != nil {
		return err
	}
	if linked {
		return errors.New("学习项目已有相关数据，无法删除")
	}
	res, err := db.Exec(`UPDATE subjects SET deleted=1,archived=0 WHERE id=? AND deleted=0`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("learning content not found")
	}

	if err := plans.SetSubjectActive(db, id, false); err != nil {
		return err
	}
	return nil
}
func hasLinkedData(db *persistence.Queries, id any) (bool, error) {
	planned, err := plans.CountForSubject(db, id)
	if err != nil {
		return false, err
	}
	recorded, err := sessions.CountForSubject(db, id)
	return planned+recorded > 0, err
}

// Count 统计科目总数。
func Count(db *persistence.Queries) (int, error) {
	var count int
	err := db.QueryRow(`SELECT count(*) FROM subjects`).Scan(&count)
	return count, err
}

func Create(db *persistence.Queries, x Subject) (Subject, error) {
	var result Subject
	err := db.WithTx(func(tx *persistence.Queries) error {
		var err error
		result, err = create(tx, x)
		return err
	})
	if err != nil {
		return x, err
	}
	return result, nil
}
