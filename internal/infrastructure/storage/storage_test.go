package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageBaseDir(t *testing.T) {
	if got := BaseDir("linux", "/opt/StudyFlow", "/home/user"); got != filepath.Join("/home/user", ".StudyFlow") {
		t.Fatalf("Linux base directory = %q", got)
	}
	if got := BaseDir("windows", `C:\Program Files\StudyFlow`, ""); got != `C:\Program Files\StudyFlow` {
		t.Fatalf("Windows base directory = %q", got)
	}
}

func TestResolveDataDirectoryMigratesOldDefault(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "install")
	base := filepath.Join(root, ".StudyFlow")
	data := filepath.Join(base, "data")
	for _, dir := range []string{install, data} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	oldDB := filepath.Join(install, "study.db")
	if err := os.WriteFile(oldDB, []byte("old database"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveDataDirectory(filepath.Join(base, "storage-location.txt"), data, install, filepath.Join(root, "old-config"))
	if err != nil {
		t.Fatal(err)
	}
	if got != data {
		t.Fatalf("data directory = %q", got)
	}
	migrated, err := os.ReadFile(filepath.Join(data, "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(migrated) != "old database" {
		t.Fatalf("migrated database = %q", migrated)
	}
	if _, err := os.Stat(oldDB); err != nil {
		t.Fatalf("old database should remain as backup: %v", err)
	}
}

func TestResolveDataDirectoryKeepsCustomChoice(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "install")
	base := filepath.Join(root, ".StudyFlow")
	custom := filepath.Join(root, "chosen")
	for _, dir := range []string{install, base, custom} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(install, "storage-location.txt"), []byte(custom+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveDataDirectory(filepath.Join(base, "storage-location.txt"), filepath.Join(base, "data"), install, filepath.Join(root, "old-config"))
	if err != nil {
		t.Fatal(err)
	}
	if got != custom {
		t.Fatalf("data directory = %q", got)
	}
	stored, err := os.ReadFile(filepath.Join(base, "storage-location.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != custom+"\n" {
		t.Fatalf("stored choice = %q", stored)
	}
}
