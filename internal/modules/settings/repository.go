package settings

import (
	"exam_notify/internal/infrastructure/persistence"
)

const upsertSetting = `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`

func listSettings(db *persistence.Queries) (map[string]string, error) {
	rows, err := db.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value

	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func readSetting(db *persistence.Queries, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	return value, err
}

func writeSettings(db *persistence.Queries, values map[string]string) error {
	for key, value := range values {
		if _, err := db.Exec(upsertSetting, key, value); err != nil {
			return err
		}
	}
	return nil
}

func writeSetting(db *persistence.Queries, key, value string) error {
	_, err := db.Exec(upsertSetting, key, value)
	return err
}
