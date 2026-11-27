package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareKeepsVersionSourcesSynchronized(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		"vue/package.json": `{"version": "1.2.3-7"}`,
		"vue/package-lock.json": `{
			"version": "1.2.3-7", "packages": {
				"": {"version": "1.2.3-7"},
				"node_modules/example": {"version": "1.2.3-7"}
			}}`,
		"build/config.yml":           "info:\n  version: \"1.2.3-7\"\n",
		"build/linux/nfpm/nfpm.yaml": "version: \"1.2.3\"\nrelease: \"7\"\n",
		"build/windows/info.json": `{
			"fixed": {"file_version": "1.2.3.7", "product_version": "1.2.3.7"},
			"info": {"0409": {"FileVersion": "1.2.3-7", "ProductVersion": "1.2.3-7"}}
		}`,
		"build/windows/nsis/project.nsi": "!define INFO_PRODUCTVERSION \"1.2.3-7\"\n!define INFO_WINDOWS_VERSION \"1.2.3.7\"\n",
	}
	for path, data := range fixtures {
		writeFixture(t, root, path, data)
	}
	if err := prepare(root); err != nil {
		t.Fatal(err)
	}
	version, err := readVersion(root)
	if err != nil || version != "1.2.3-8" {
		t.Fatalf("版本错误: %s, %v", version, err)
	}
	lockData, _ := os.ReadFile(filepath.Join(root, "vue/package-lock.json"))
	var lock struct {
		Version  string
		Packages map[string]struct{ Version string }
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.Version != version || lock.Packages[""].Version != version || lock.Packages["node_modules/example"].Version != "1.2.3-7" {
		t.Fatalf("锁文件改变依赖版本或未同步: %s", lockData)
	}
	for path, want := range map[string]string{
		"build/config.yml":               "1.2.3-8",
		"build/linux/nfpm/nfpm.yaml":     "release: \"8\"",
		"build/windows/nsis/project.nsi": "1.2.3.8",
	} {
		data, _ := os.ReadFile(filepath.Join(root, path))
		if !strings.Contains(string(data), want) {
			t.Errorf("配置未同步: %s", path)
		}
	}
	windows, err := os.ReadFile(filepath.Join(root, "build/windows/info.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{
		`"file_version": "1.2.3.8"`, `"product_version": "1.2.3.8"`,
		`"FileVersion": "1.2.3-8"`, `"ProductVersion": "1.2.3-8"`,
	} {
		if !strings.Contains(string(windows), entry) {
			t.Errorf("Windows 版本未同步: %s", entry)
		}
	}
	if err := prepare(root); err != nil {
		t.Fatal(err)
	}
	if version, _ := readVersion(root); version != "1.2.3-9" {
		t.Fatalf("再次构建未递增: %s", version)
	}
}

func TestFinalizeOnlyRemovesMatchingBuildArtifacts(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"vue/package.json":                        `{"version": "1.2.3-8"}`,
		"bin/StudyFlow":                           "new executable",
		"bin/StudyFlow-1.2.3-7-linux-amd64":       "old executable",
		"bin/StudyFlow-1.2.3-7-windows-amd64.exe": "windows executable",
		"bin/StudyFlow-1.2.3-7-linux-arm64":       "arm executable",
		"bin/study.db":                            "user database",
	}
	for name, data := range files {
		writeFixture(t, root, name, data)
	}
	if err := finalize(root, "binary", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "bin/StudyFlow-1.2.3-7-linux-amd64")); !os.IsNotExist(err) {
		t.Fatalf("旧产物未删除: %v", err)
	}
	for _, path := range []string{
		"bin/StudyFlow", "bin/StudyFlow-1.2.3-8-linux-amd64",
		"bin/StudyFlow-1.2.3-7-windows-amd64.exe", "bin/StudyFlow-1.2.3-7-linux-arm64", "bin/study.db",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("文件被意外删除: %s: %v", path, err)
		}
	}
}

func TestLinuxPackagesRemoveLegacyNamesAndPreserveOtherArchitectures(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "vue/package.json", `{"version": "1.2.3-8"}`)
	for _, extension := range []string{"AppImage", "deb", "rpm", "pkg.tar.zst"} {
		writeFixture(t, root, "bin/StudyFlow."+extension, "new package")
		writeFixture(t, root, "bin/StudyFlow-1.2.3-7-linux-amd64."+extension, "old package")
		writeFixture(t, root, "bin/StudyFlow-1.2.3-7-amd64."+extension, "legacy package")
		writeFixture(t, root, "bin/StudyFlow-1.2.3-7-linux-arm64."+extension, "other architecture")
	}
	if err := finalize(root, "packages", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, extension := range []string{"AppImage", "deb", "rpm", "pkg.tar.zst"} {
		for _, prefix := range []string{"StudyFlow-1.2.3-8-linux-amd64", "StudyFlow-1.2.3-7-linux-arm64"} {
			if _, err := os.Stat(filepath.Join(root, "bin", prefix+"."+extension)); err != nil {
				t.Errorf("需要保留的安装包丢失: %s: %v", prefix, err)
			}
		}
		for _, prefix := range []string{"StudyFlow-1.2.3-7-linux-amd64", "StudyFlow-1.2.3-7-amd64"} {
			if _, err := os.Stat(filepath.Join(root, "bin", prefix+"."+extension)); !os.IsNotExist(err) {
				t.Errorf("旧安装包未清理: %s: %v", prefix, err)
			}
		}
	}
}
