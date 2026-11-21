package reminder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRingtoneResourcePreservedAndRepaired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ringtone.mp3")
	data := []byte("test ringtone")
	if err := writeRingtone(path, data); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1000000000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := writeRingtone(path, data); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Fatal("重复启动改写了相同资源")
	}
	if err := os.WriteFile(path, []byte("bad ringtone!"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeRingtone(path, data); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(data) {
		t.Fatal("损坏资源未恢复")
	}
}
