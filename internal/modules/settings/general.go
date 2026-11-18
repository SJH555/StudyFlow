package settings

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"strconv"
)

// GeneralConfig 保存常规设置：铃声大小、启动后窗口行为与关闭按钮行为。
type GeneralConfig struct {
	RingtoneVolume int    // 铃声音量 0–100，0 为静音。
	StartupWindow  string // "show" 启动后显示主窗口，"tray" 启动后驻留托盘。
	CloseBehavior  string // "tray" 关闭时隐藏到托盘，"quit" 关闭时退出应用。
}

// ReadGeneral 读取常规设置，未配置的项返回默认值（音量 100、显示窗口、隐藏到托盘）。
func ReadGeneral(db *persistence.Queries) (GeneralConfig, error) {
	rows, err := db.Query(`SELECT key,value FROM settings WHERE key IN ('ringtoneVolume','startupWindow','closeBehavior')`)
	if err != nil {
		return GeneralConfig{}, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return GeneralConfig{}, err
		}
		values[key] = value
	}
	if err = rows.Err(); err != nil {
		return GeneralConfig{}, err
	}
	config := GeneralConfig{RingtoneVolume: 100, StartupWindow: "show", CloseBehavior: "tray"}
	if volume, convErr := strconv.Atoi(values["ringtoneVolume"]); convErr == nil && volume >= 0 && volume <= 100 {
		config.RingtoneVolume = volume
	}
	if values["startupWindow"] == "tray" {
		config.StartupWindow = "tray"
	}
	if values["closeBehavior"] == "quit" {
		config.CloseBehavior = "quit"
	}
	return config, nil
}

// CheckHour 返回进度追因检查的日界小时（0–23），未设置或非法时默认 6 点。
func CheckHour(db *persistence.Queries) (int, error) {
	value, err := readSetting(db, "dailyReviewCheckHour")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 6, nil
		}
		return 6, err
	}
	hour, convErr := strconv.Atoi(value)
	if convErr != nil || hour < 0 || hour > 23 {
		return 6, nil
	}
	return hour, nil
}

// CheckMinute 返回进度追因检查的分钟（0–59），未设置或非法时默认 0 分。
func CheckMinute(db *persistence.Queries) (int, error) {
	value, err := readSetting(db, "dailyReviewCheckMinute")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	minute, convErr := strconv.Atoi(value)
	if convErr != nil || minute < 0 || minute > 59 {
		return 0, nil
	}
	return minute, nil
}
