//go:build windows

package reminder

import "testing"

func TestWindowsAudioWorkerStopsAndCanBeRecreated(t *testing.T) {
	if err := mciCommand("sysinfo all quantity"); err != nil {
		t.Fatal(err)
	}
	shutdownWindowsAudio()
	mciMu.Lock()
	requests, done := mciRequests, mciWorkerDone
	mciMu.Unlock()
	if requests != nil {
		t.Fatal("audio worker still accepts commands")
	}
	select {
	case <-done:
	default:
		t.Fatal("audio thread did not finish")
	}
	shutdownWindowsAudio()
	if err := mciCommand("sysinfo all quantity"); err != nil {
		t.Fatal(err)
	}
	shutdownWindowsAudio()
}
