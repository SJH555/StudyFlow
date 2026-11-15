//go:build !windows

package reminder

func playWindowsRingtone(string, int) error { return nil }
func stopWindowsRingtone()                  {}

func shutdownWindowsAudio() {}
