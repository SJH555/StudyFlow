package studytime

import "time"

// Elapsed 计算学习时段已用时：暂停时返回累计值，否则从起点或恢复时刻累加。
func Elapsed(start string, resumed *string, paused bool, stored int64, now time.Time) int64 {
	if paused {
		return stored
	}
	base := start
	if resumed != nil {
		base = *resumed
	}
	from, err := time.Parse(time.RFC3339, base)
	if err != nil {
		return stored
	}
	seconds := int64(now.Sub(from).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return stored + seconds
}

// BreakElapsedSeconds 计算从休息开始到 now 的休息时长（秒）。
func BreakElapsedSeconds(started *string, now time.Time) int64 {
	if started == nil {
		return 0
	}
	start, err := time.Parse(time.RFC3339Nano, *started)
	if err != nil {
		return 0
	}
	return max(0, int64(now.Sub(start).Seconds()))
}
