package settings

import (
	"exam_notify/internal/infrastructure/persistence"
	"exam_notify/internal/modules/schedule"
)

func resettableNames(db *persistence.Queries) ([]string, error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return nil, err
	}
	query := `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`
	if history {
		query += ` UNION SELECT 'subjects' UNION SELECT 'plans' UNION SELECT 'daily_plan_targets'`
	}
	rows, err := db.Query(query + " ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if _, ok := resetTableLabels[name]; ok {
			names = append(names, name)
		}

	}
	return names, rows.Err()
}

func resetTableCount(db *persistence.Queries, allowedName string) (int64, error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return 0, err
	}
	query := `SELECT count(*) FROM "` + allowedName + `"`
	if history {
		switch allowedName {
		case "subjects":
			query = `SELECT count(DISTINCT subject_id) FROM subject_versions`
		case "plans":
			query = `SELECT count(DISTINCT plan_id) FROM plan_versions WHERE kind='recurring'`
		case "daily_plan_targets":
			query = `SELECT count(*) FROM plan_versions WHERE kind='legacy_target'`
		}
	}
	var count int64
	err = db.QueryRow(query).Scan(&count)
	return count, err
}

func subjectResetLinks(db *persistence.Queries) (int, error) {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return 0, err
	}
	query := `SELECT (SELECT count(*) FROM plans) + (SELECT count(*) FROM sessions)`
	if history {
		query = `SELECT (SELECT count(*) FROM plan_versions WHERE kind='recurring') + (SELECT count(*) FROM sessions)`
	}
	var count int
	err = db.QueryRow(query).Scan(&count)
	return count, err
}

func clearResetTable(db *persistence.Queries, allowedName string) error {
	history, err := schedule.HasHistory(db)
	if err != nil {
		return err
	}
	query := `DELETE FROM "` + allowedName + `"`
	if history {
		switch allowedName {
		case "subjects":
			query = `DELETE FROM subject_versions`
		case "plans":
			query = `DELETE FROM plan_versions WHERE kind='recurring'`
		case "daily_plan_targets":
			query = `DELETE FROM plan_versions WHERE kind='legacy_target'`
		}
	}
	_, err = db.Exec(query)
	return err
}
