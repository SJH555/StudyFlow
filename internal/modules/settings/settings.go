package settings

import (
	"database/sql"
	"errors"
	"exam_notify/internal/infrastructure/persistence"
	"strconv"
	"time"
)

// Actions 保存设置变更后需要触发的回调。
type Actions struct {
	ActivityReminderChanged func(bool)
	ReviewDisabled          func()
	BeforeReviewRulesChange func(*persistence.Queries, map[string]string) error
	BackupEnabled           func()
	// GeneralChanged 在常规设置（音量、启动窗口、关闭行为、检查时间）变化后触发。
	GeneralChanged func()
}

// List 返回全部设置键值。
func List(db *persistence.Queries) (map[string]string, error) {
	return listSettings(db)
}

// Save 校验并保存设置，并在必要时触发相应回调。
func Save(db *persistence.Queries, in map[string]string, actions Actions) error {
	if err := validateGeneral(in); err != nil {
		return err
	}
	if err := validateNavigationWidths(in); err != nil {
		return err
	}
	var previousBackup string
	err := db.WithTx(func(db *persistence.Queries) error {
		if enabled, ok := in["dailyReviewEnabled"]; ok {
			if enabled != "0" && enabled != "1" {
				return errors.New("总结开关状态无效")
			}
			if enabled == "1" {
				previous, err := readSetting(db, "dailyReviewEnabled")
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if previous != "1" {
					// 新开启的周期从今天开始；编辑其他设置不会改变其起始日期。
					in["dailyReviewStartDate"] = time.Now().Format("2006-01-02")
				}
			}
		}
		if _, ok := in[autoBackupEnabledKey]; ok {
			var err error
			previousBackup, err = readSetting(db, autoBackupEnabledKey)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		_, changesScope := in["dailyReviewMode"]
		_, changesThreshold := in["dailyReviewThreshold"]
		if actions.BeforeReviewRulesChange != nil && (changesScope || changesThreshold) {
			if err := actions.BeforeReviewRulesChange(db, in); err != nil {
				return err
			}
		}
		if err := writeSettings(db, in); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if value, ok := in[activityReminderEnabledKey]; ok && actions.ActivityReminderChanged != nil {
		actions.ActivityReminderChanged(value == "1")
	}
	if in["dailyReviewEnabled"] == "0" && actions.ReviewDisabled != nil {
		actions.ReviewDisabled()
	}
	if in[autoBackupEnabledKey] == "1" && previousBackup != "1" && actions.BackupEnabled != nil {
		actions.BackupEnabled()
	}
	if actions.GeneralChanged != nil {
		for _, key := range []string{"ringtoneVolume", "startupWindow", "closeBehavior", "dailyReviewCheckHour", "dailyReviewCheckMinute"} {
			if _, ok := in[key]; ok {
				actions.GeneralChanged()
				break
			}
		}
	}
	return nil
}

// validateGeneral 校验常规设置取值范围：音量 0–100，检查小时 0–23，
// 启动窗口与关闭行为只允许预设枚举。
func validateGeneral(in map[string]string) error {
	if value, ok := in["ringtoneVolume"]; ok {
		volume, err := strconv.Atoi(value)
		if err != nil || volume < 0 || volume > 100 {
			return errors.New("铃声音量必须是 0–100 的整数")
		}
	}
	if value, ok := in["dailyReviewCheckHour"]; ok {
		hour, err := strconv.Atoi(value)
		if err != nil || hour < 0 || hour > 23 {
			return errors.New("检查时间必须是 0–23 的小时")
		}
	}
	if value, ok := in["dailyReviewCheckMinute"]; ok {
		minute, err := strconv.Atoi(value)
		if err != nil || minute < 0 || minute > 59 {
			return errors.New("检查分钟必须是 0–59 的整数")
		}
	}
	if value, ok := in["startupWindow"]; ok && value != "show" && value != "tray" {
		return errors.New("启动窗口行为无效")
	}
	if value, ok := in["closeBehavior"]; ok && value != "tray" && value != "quit" {
		return errors.New("关闭按钮行为无效")
	}
	return nil
}
