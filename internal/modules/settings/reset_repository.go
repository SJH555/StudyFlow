package settings

import (
	"exam_notify/internal/infrastructure/persistence"
)

func resettableNames(db *persistence.Queries) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
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
	var count int64
	err := db.QueryRow(`SELECT count(*) FROM "` + allowedName + `"`).Scan(&count)
	return count, err
}

func subjectResetLinks(db *persistence.Queries) (int, error) {
	var count int
	err := db.QueryRow(`SELECT (SELECT count(*) FROM plans) + (SELECT count(*) FROM sessions)`).Scan(&count)
	return count, err
}

func clearResetTable(db *persistence.Queries, allowedName string) error {
	_, err := db.Exec(`DELETE FROM "` + allowedName + `"`)
	return err
}
