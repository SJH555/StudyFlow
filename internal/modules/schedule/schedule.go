package schedule

import (
	"fmt"
	"strings"
	"time"
)

// WeekdayLabels 是星期标签，下标与 time.Weekday 一致。
var WeekdayLabels = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// ContainsWeekday 判断以逗号分隔的星期串中是否包含指定星期。
func ContainsWeekday(csv string, weekday int) bool {
	for _, part := range strings.Split(csv, ",") {
		if part == fmt.Sprint(weekday) {
			return true
		}
	}
	return false
}

// ParseWeekdays 解析逗号分隔的星期串为 7 天布尔数组。
func ParseWeekdays(value string) ([7]bool, error) {
	var days [7]bool
	if value == "" {
		return days, fmt.Errorf("请至少选择一个重复星期")
	}
	for _, part := range strings.Split(value, ",") {
		if len(part) != 1 || part[0] < '0' || part[0] > '6' {
			return days, fmt.Errorf("重复星期无效")
		}
		days[part[0]-'0'] = true
	}
	return days, nil
}

// StartMinute 将 "HH:MM" 解析为当天的分钟数。
func StartMinute(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("开始时间无效")
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// Overlap 判断两个计划时段在任意重叠的星期是否重叠，并返回发生重叠的星期。
func Overlap(aStart, aDuration int, aDays [7]bool, bStart, bDuration int, bDays [7]bool) (int, bool) {
	const week = 7 * 24 * 60
	for aDay, selected := range aDays {
		if !selected {
			continue
		}
		aFrom := aDay*1440 + aStart
		for bDay, otherSelected := range bDays {
			if !otherSelected {
				continue
			}
			for _, shift := range []int{-week, 0, week} {
				bFrom := bDay*1440 + bStart + shift
				if aFrom < bFrom+bDuration && bFrom < aFrom+aDuration {
					return aDay, true
				}
			}
		}
	}
	return 0, false
}

// ScheduledEnd 返回计时器开始时刻所在的计划时段结束时间。
// 同时检查昨天，以覆盖跨午夜的计划。
func ScheduledEnd(moment time.Time, startTime string, duration int, weekdays string) (time.Time, bool) {
	clock, err := time.Parse("15:04", startTime)
	if err != nil || duration < 1 {
		return time.Time{}, false
	}
	local := moment.Local()
	for _, offset := range []int{0, -1} {
		day := local.AddDate(0, 0, offset)
		start := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, local.Location())
		end := start.Add(time.Duration(duration) * time.Minute)
		if ContainsWeekday(weekdays, int(day.Weekday())) && !local.Before(start) && local.Before(end) {
			return end, true
		}
	}
	return time.Time{}, false
}

// BoundaryEnd 返回计时开始时刻对应的计划时段结束时间：moment 落在时段内时返回该时段结束；
// moment 早于当天时段开始时返回当天该时段的结束（提前开始同样锚定到计划结束时刻）。
// 其余情况返回 false，调用方保持无边界（不自动结束）的既有语义。
func BoundaryEnd(moment time.Time, startTime string, duration int, weekdays string) (time.Time, bool) {
	if end, ok := ScheduledEnd(moment, startTime, duration, weekdays); ok {
		return end, true
	}
	clock, err := time.Parse("15:04", startTime)
	if err != nil || duration < 1 {
		return time.Time{}, false
	}
	local := moment.Local()
	if !ContainsWeekday(weekdays, int(local.Weekday())) {
		return time.Time{}, false
	}
	start := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, local.Location())
	if !moment.Before(start) {
		return time.Time{}, false
	}
	return start.Add(time.Duration(duration) * time.Minute), true
}
