package subjects

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/plans"
	"exam_notify/internal/modules/schedule"
	"exam_notify/internal/shared/studycalendar"
)

const subjectAt = `FROM (SELECT DISTINCT subject_id FROM subject_versions) i JOIN subject_versions v ON v.id=(
	SELECT sv.id FROM subject_versions sv WHERE sv.subject_id=i.subject_id AND sv.effective_day<=?
	ORDER BY sv.effective_day DESC,sv.id DESC LIMIT 1
)`

// AsOf 按同一个版本索引读取当前、历史和待生效的科目。
func AsOf(db *persistence.Queries, day string, includeStopped, includeDeleted bool) ([]Subject, error) {
	rows, err := db.Query(`SELECT i.subject_id,v.name,v.color,v.sort_order,v.status,
		EXISTS(SELECT 1 FROM plan_versions p WHERE p.kind='recurring' AND p.subject_id=i.subject_id)
		OR EXISTS(SELECT 1 FROM sessions s WHERE s.subject_id=i.subject_id AND s.deleted=0) `+
		subjectAt+` ORDER BY v.status='deleted',v.status='stopped',v.sort_order IS NULL,
		v.sort_order,v.created_at,i.subject_id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Subject, 0)
	for rows.Next() {
		var item Subject
		var status string
		if err := rows.Scan(&item.ID, &item.Name, &item.Color, &item.SortOrder, &status, &item.HasData); err != nil {
			return nil, err
		}
		item.Archived, item.Deleted = status == plans.Stopped, status == plans.Deleted
		if (item.Archived && !includeStopped) || (item.Deleted && !includeDeleted) {
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ManualOptions 读取补录日期的项目。升级前没有独立的项目版本，沿用升级日已知的状态。
func ManualOptions(db *persistence.Queries, day string) ([]Subject, error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return nil, err
	}
	if !history {
		return List(db, false, false)
	}
	var adoption sql.NullString
	if err := db.QueryRow(`SELECT adoption_day FROM plan_versions WHERE kind='history_boundary'`).Scan(&adoption); err != nil {
		return nil, err
	}
	if adoption.Valid && day < adoption.String {
		day = adoption.String
	}
	return AsOf(db, day, false, false)
}

func validateVersion(item *Subject) error {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || utf8.RuneCountInString(item.Name) > 50 {
		return errors.New("学习项目名称需为 1 到 50 字")
	}
	if item.Color == "" {
		item.Color = "#007AFF"
	}
	if item.SortOrder != nil && *item.SortOrder < 1 {
		return errors.New("排序号需为正整数")
	}
	return nil
}

func checkName(tx *persistence.Queries, item Subject, day string) error {
	var exists bool
	err := tx.QueryRow(`SELECT EXISTS(SELECT 1 `+subjectAt+`
		WHERE v.name=? AND i.subject_id<>? AND v.status<>'deleted')`,
		day, item.Name, item.ID).Scan(&exists)
	if err == nil && exists {
		return errors.New("已有同名学习项目")
	}
	return err
}

func appendSubject(tx *persistence.Queries, item Subject, day, createdAt, status string) error {
	_, err := tx.Exec(`INSERT INTO subject_versions(subject_id,effective_day,name,color,sort_order,created_at,status)
		VALUES(?,?,?,?,?,?,?)`, item.ID, day, item.Name, item.Color, item.SortOrder, createdAt, status)
	return err
}

// SaveVersion 新科目当天可补录，已有科目的编辑和停用从次日生效。
func SaveVersion(db *persistence.Queries, item Subject, create bool, now time.Time) (Subject, error) {
	if err := validateVersion(&item); err != nil {
		return item, err
	}
	day := studycalendar.Tomorrow(now)
	err := db.WithTx(func(tx *persistence.Queries) error {
		if err := checkName(tx, item, day); err != nil {
			return err
		}
		status := plans.Active
		createdAt := now.Format(time.RFC3339)
		if create {
			item.ID = 0
			day = studycalendar.Day(now)
			if err := checkName(tx, item, day); err != nil {
				return err
			}
			if err := tx.QueryRow(`SELECT COALESCE(MAX(subject_id),0)+1 FROM subject_versions`).Scan(&item.ID); err != nil {
				return err
			}
		} else {
			var previous string
			err := tx.QueryRow(`SELECT status,created_at FROM subject_versions WHERE subject_id=? AND effective_day<=?
				ORDER BY effective_day DESC,id DESC LIMIT 1`, item.ID, day).Scan(&previous, &createdAt)
			if errors.Is(err, sql.ErrNoRows) || previous == plans.Deleted {
				return errors.New("学习项目不存在或已删除")
			}
			if err != nil {
				return err
			}
		}
		if item.Archived {
			status = plans.Stopped
		}
		if item.Deleted {
			return errors.New("请使用永久删除操作")
		}
		if err := appendSubject(tx, item, day, createdAt, status); err != nil {
			return err
		}
		if !create && status == plans.Active {
			return plans.ValidateSubjectSchedule(tx, item.ID, day)
		}
		return nil
	})
	return item, err
}

// DeleteVersion 保留科目身份、计划版本及学习记录，仅从次日停止使用。
func DeleteVersion(db *persistence.Queries, id string, now time.Time) error {
	day := studycalendar.Tomorrow(now)
	return db.WithTx(func(tx *persistence.Queries) error {
		result, err := tx.Exec(`INSERT INTO subject_versions(subject_id,effective_day,name,color,sort_order,created_at,status)
			SELECT subject_id,?,name,color,sort_order,created_at,'deleted' FROM subject_versions
			WHERE id=(SELECT id FROM subject_versions WHERE subject_id=? AND effective_day<=?
			ORDER BY effective_day DESC,id DESC LIMIT 1) AND status<>'deleted'`, day, id, day)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err == nil && count != 1 {
			return errors.New("学习项目不存在或已删除")
		}
		return err
	})
}
