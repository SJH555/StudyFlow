package settings

import (
	"exam_notify/internal/infrastructure/persistence"

	"strconv"
)

// ReviewConfig 返回复习相关配置。
func ReviewConfig(db *persistence.Queries) (bool, string, int, string, error) {
	values := map[string]string{}
	rows, err := db.Query(`SELECT key,value FROM settings WHERE key IN ('dailyReviewEnabled','dailyReviewMode','dailyReviewThreshold','dailyReviewStartDate')`)
	if err != nil {
		return false, "", 0, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return false, "", 0, "", err
		}
		values[key] = value
	}
	if err = rows.Err(); err != nil {
		return false, "", 0, "", err
	}
	mode := values["dailyReviewMode"]
	if mode != "project" {
		mode = "total"
	}
	threshold, err := strconv.Atoi(values["dailyReviewThreshold"])
	if err != nil || threshold < 1 || threshold > 100 {
		threshold = 50
	}
	return values["dailyReviewEnabled"] == "1", mode, threshold, values["dailyReviewStartDate"], nil
}
