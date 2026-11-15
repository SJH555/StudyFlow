//go:build windows

package reminder

import "testing"

func TestWindowsRingtoneClosesAfterAcknowledgement(t *testing.T) {
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	Start(ActivitySound)
	if !player.windowsPlaying {
		t.Skip("Windows media device is unavailable")
	}
	Stop()
	if err := mciCommand("status " + reminderMCIAlias + " mode"); err == nil {
		t.Fatal("reminder media device remained open after stop")
	}
}
