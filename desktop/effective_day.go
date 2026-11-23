package main

import (
	"net/http"
	"time"

	"exam_notify/internal/shared/studycalendar"
)

// The same clock value is passed to the write so the confirmed day cannot change mid-request.
func confirmEffectiveDay(w http.ResponseWriter, r *http.Request, now time.Time) bool {
	expected := r.URL.Query().Get("expectedEffectiveDay")
	effective := studycalendar.Tomorrow(now)
	if expected == "" || expected == effective {
		return true
	}
	writeJSON(w, map[string]any{"needsConfirmation": true, "effectiveDay": effective})
	return false
}
