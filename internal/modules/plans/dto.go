package plans

// Plan 表示一条学习计划。
type Plan struct {
	ID           int64  `json:"id"`
	SubjectID    int64  `json:"subjectId"`
	Subject      string `json:"subject"`
	Color        string `json:"color"`
	StartTime    string `json:"startTime"`
	Duration     int    `json:"duration"`
	BreakMinutes int    `json:"breakMinutes"`
	Weekdays     string `json:"weekdays"`
	Active       bool   `json:"active"`
	Note         string `json:"note"`
}
