//go:build linux

package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledLinuxLauncherUsesUserDataDirectory(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	if err := EnsureUserLauncher("/usr/local/bin/StudyFlow", []byte("test icon")); err != nil {
		t.Fatal(err)
	}
	desktop, err := os.ReadFile(filepath.Join(dataHome, "applications", "org.wails.studyflow.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Exec=/usr/local/bin/StudyFlow", "TryExec=/usr/local/bin/StudyFlow", "Icon=" + filepath.Join(dataHome, "icons", "hicolor", "1024x1024", "apps", "org.wails.studyflow.png")} {
		if !strings.Contains(string(desktop), expected) {
			t.Fatalf("missing %q in launcher", expected)
		}
	}
	icon, err := os.ReadFile(filepath.Join(dataHome, "icons", "hicolor", "1024x1024", "apps", "org.wails.studyflow.png"))
	if err != nil || string(icon) != "test icon" {
		t.Fatalf("user icon: %q, %v", icon, err)
	}
}

func TestDevelopmentBinaryDoesNotOverrideInstalledLauncher(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	if err := EnsureUserLauncher("/home/user/project/bin/StudyFlow", []byte("icon")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataHome, "applications", "org.wails.studyflow.desktop")); !os.IsNotExist(err) {
		t.Fatalf("development binary registered a launcher: %v", err)
	}
}
