package studycalendar

import (
	"time"
	_ "time/tzdata"
)

const ZoneID = "Asia/Shanghai"

var Location = loadLocation()

func loadLocation() *time.Location {
	location, err := time.LoadLocation(ZoneID)
	if err != nil {
		panic(err) // 标准库已内置时区数据。
	}
	return location
}

func Day(now time.Time) string {
	return now.In(Location).Format(time.DateOnly)
}

func Tomorrow(now time.Time) string {
	return now.In(Location).AddDate(0, 0, 1).Format(time.DateOnly)
}

// IsSupported 识别 Windows 时区键名和 IANA 的北京时间别名。
func IsSupported(id string) bool {
	switch id {
	case ZoneID, "China Standard Time", "PRC", "Asia/Chongqing", "Asia/Chungking", "Asia/Harbin":
		return true
	default:
		return false
	}
}
