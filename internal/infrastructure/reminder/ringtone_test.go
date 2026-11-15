package reminder

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRingtonesUseSeparateFiles(t *testing.T) {
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	activity, err := os.ReadFile(player.files[ActivitySound])
	if err != nil {
		t.Fatal(err)
	}
	plan, err := os.ReadFile(player.files[PlanSound])
	if err != nil {
		t.Fatal(err)
	}
	breakAudio, err := os.ReadFile(player.files[BreakSound])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(activity, activityRingtoneData) || !bytes.Equal(plan, planRingtoneData) || !bytes.Equal(breakAudio, breakRingtoneData) {
		t.Fatal("reminder sound files do not match embedded assets")
	}
	if filepath.Clean(player.files[ActivitySound]) == filepath.Clean(player.files[PlanSound]) || filepath.Clean(player.files[BreakSound]) == filepath.Clean(player.files[PlanSound]) {
		t.Fatal("reminder sounds share a file")
	}
}
