package main

import (
	"path/filepath"
	"testing"
)

func TestActivityReminderTogglePersistsAndNotifies(t *testing.T) {
	store, err := NewApp(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var states []bool
	store.onActivityReminderChanged = func(enabled bool) { states = append(states, enabled) }
	for _, want := range []bool{true, false} {
		if err := store.setActivityReminderEnabled(want); err != nil {
			t.Fatal(err)
		}
		got, err := store.activityReminderEnabled()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("reminder state = %t, want %t", got, want)
		}
	}
	if len(states) != 2 || !states[0] || states[1] {
		t.Fatalf("notifications = %v", states)
	}
}
