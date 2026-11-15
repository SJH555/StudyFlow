package studytime

import (
	"testing"
	"time"
)

func TestElapsedPreservesOptionalTimestamps(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 3, 900000000, time.UTC)
	start := "2026-11-05T12:00:00Z"
	resumed := "2026-11-05T12:00:02.100Z"
	empty := ""
	future := "2026-11-05T12:00:08Z"
	for _, tc := range []struct {
		name    string
		resumed *string
		paused  bool
		want    int64
	}{
		{"absent uses start and truncates", nil, false, 13},
		{"resumed takes precedence", &resumed, false, 11},
		{"present empty does not fall back", &empty, false, 10},
		{"paused returns stored", &resumed, true, 10},
		{"negative time clamps", &future, false, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Elapsed(start, tc.resumed, tc.paused, 10, now); got != tc.want {
				t.Fatalf("Elapsed = %d; want %d", got, tc.want)
			}
		})
	}
}

func TestBreakElapsedSecondsHandlesInvalidAndFractionalTime(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 3, 900000000, time.UTC)
	for _, tc := range []struct {
		started *string
		want    int64
	}{
		{nil, 0},
		{timestamp(""), 0},
		{timestamp("invalid"), 0},
		{timestamp("2026-11-05T12:00:09Z"), 0},
		{timestamp("2026-11-05T12:00:02.100000001Z"), 1},
	} {
		if got := BreakElapsedSeconds(tc.started, now); got != tc.want {
			t.Fatalf("BreakElapsedSeconds = %d; want %d", got, tc.want)
		}
	}
}

func timestamp(value string) *string { return &value }
