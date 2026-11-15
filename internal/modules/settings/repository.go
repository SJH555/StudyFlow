package settings

import "database/sql"

const upsertSetting = `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`

func listSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) == nil {
			out[key] = value
		}
	}
	return out, nil
}

func readSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	return value, err
}

func writeSettings(db *sql.DB, values map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range values {
		if _, err := tx.Exec(upsertSetting, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func writeSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(upsertSetting, key, value)
	return err
}
