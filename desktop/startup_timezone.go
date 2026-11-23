package main

import (
	"log"

	"exam_notify/internal/infrastructure/system"
	"exam_notify/internal/shared/studycalendar"
)

func checkStartupTimeZone() *startupStatus {
	id, err := system.SystemTimeZone()
	if err != nil || id == "" {
		log.Printf("无法读取系统时区: %v", err)
		return &startupStatus{Phase: "blocked", Kind: "timezone-unknown",
			Message: "无法确认系统时区。请在系统设置中选择北京时间，然后重新打开 StudyFlow。"}
	}
	if studycalendar.IsSupported(id) {
		return nil
	}
	return &startupStatus{Phase: "blocked", Kind: "timezone-unsupported", SystemZone: id,
		Message: "StudyFlow 目前仅支持北京时间。请在系统设置中修改时区，然后重新打开 StudyFlow。"}
}
