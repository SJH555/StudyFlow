package plans

// Plan 表示一条学习计划。
type Plan struct {
	ID            int64   `json:"id"`
	VersionID     int64   `json:"versionId,omitempty"`
	EffectiveDay  string  `json:"effectiveDay,omitempty"`
	Unchanged     bool    `json:"unchanged,omitempty"`
	Status        string  `json:"status,omitempty"`
	ManagedActive bool    `json:"managedActive"`
	PendingStatus bool    `json:"pendingStatus,omitempty"`
	Pending       bool    `json:"pending,omitempty"`
	PendingLabel  string  `json:"pendingLabel,omitempty"`
	PendingDay    string  `json:"pendingDay,omitempty"`
	SubjectID     int64   `json:"subjectId"`
	Subject       string  `json:"subject"`
	Color         string  `json:"color"`
	ColorOverride *string `json:"colorOverride,omitempty"` // 空字符串表示随项目颜色变化。
	StartTime     string  `json:"startTime"`
	Duration      int     `json:"duration"`
	BreakMinutes  int     `json:"breakMinutes"`
	Weekdays      string  `json:"weekdays"`
	Active        bool    `json:"active"`
	Note          string  `json:"note"`
}

func (p Plan) storedColor() string {
	if p.ColorOverride != nil {
		return *p.ColorOverride
	}
	return p.Color
}
