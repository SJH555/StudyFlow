//go:build windows

package reminder

import "testing"

func TestWindowsQAAudioShutdownAndRecreate(t *testing.T) {
	for i := 0; i < 2; i++ {
		if err := Init(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []Sound{ActivitySound, PlanSound, BreakSound} {
			Start(kind)
			player.mu.Lock()
			playing := player.windowsPlaying
			player.mu.Unlock()
			if !playing {
				t.Fatalf("%s did not start", kind)
			}
			Stop()
		}
		Start(PlanSound)
		Shutdown()
		mciMu.Lock()
		requests, done := mciRequests, mciWorkerDone
		mciMu.Unlock()
		if requests != nil {
			t.Fatal("worker still accepts commands")
		}
		select {
		case <-done:
		default:
			t.Fatal("worker did not terminate")
		}
		Shutdown()
	}
}
