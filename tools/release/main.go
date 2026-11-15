// Command release 同步构建版本，并整理成功构建的当前平台产物。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-(\d+))?$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "prepare" {
		return prepare(".")
	}
	if len(args) != 3 || (args[0] != "binary" && args[0] != "packages") {
		return errors.New("用法: release prepare | release binary|packages <系统> <架构>")
	}
	return finalize(".", args[0], args[1], args[2])
}

func readVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "vue/package.json"))
	if err != nil {
		return "", err
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", err
	}
	if !versionPattern.MatchString(info.Version) {
		return "", fmt.Errorf("无效的应用版本: %q", info.Version)
	}
	return info.Version, nil
}

func nextVersion(current string) (full, base, windows string, err error) {
	parts := versionPattern.FindStringSubmatch(current)
	if parts == nil {
		return "", "", "", fmt.Errorf("无效的应用版本: %q", current)
	}
	build := 0
	if parts[4] != "" {
		build, err = strconv.Atoi(parts[4])
		if err != nil {
			return "", "", "", err
		}
	}
	// Windows 文件版本的每一段均为 16 位整数。
	if build >= 65535 {
		return "", "", "", errors.New("构建号已达到 Windows 文件版本上限")
	}
	base = strings.Join(parts[1:4], ".")
	build++
	return fmt.Sprintf("%s-%d", base, build), base, fmt.Sprintf("%s.%d", base, build), nil
}

type replacement struct {
	path    string
	pattern string
	value   string
	count   int
}

func prepare(root string) error {
	current, err := readVersion(root)
	if err != nil {
		return err
	}
	full, base, windows, err := nextVersion(current)
	if err != nil {
		return err
	}
	build := strings.Split(full, "-")[1]
	changes := []replacement{
		{
			path:    "vue/package.json",
			pattern: `"version": "[^"]+"`,
			value:   `"version": "` + full + `"`,
			count:   1,
		},
		{
			path:    "vue/package-lock.json",
			pattern: `"version": "` + regexp.QuoteMeta(current) + `"`,
			value:   `"version": "` + full + `"`,
			count:   2,
		},
		{
			path:    "build/config.yml",
			pattern: `(?m)^  version: "[^"]+"`,
			value:   `  version: "` + full + `"`,
			count:   1,
		},
		{
			path:    "build/linux/nfpm/nfpm.yaml",
			pattern: `(?m)^version: "[^"]+"`,
			value:   `version: "` + base + `"`,
			count:   1,
		},
		{
			path:    "build/linux/nfpm/nfpm.yaml",
			pattern: `(?m)^release: "[^"]+"`,
			value:   `release: "` + build + `"`,
			count:   1,
		},
		{
			path:    "build/windows/info.json",
			pattern: `"file_version": "[^"]+"`,
			value:   `"file_version": "` + windows + `"`,
			count:   1,
		},
		{
			path:    "build/windows/info.json",
			pattern: `"ProductVersion": "[^"]+"`,
			value:   `"ProductVersion": "` + full + `"`,
			count:   1,
		},
		{
			path:    "build/windows/nsis/project.nsi",
			pattern: `(?m)^!define INFO_PRODUCTVERSION "[^"]+"`,
			value:   `!define INFO_PRODUCTVERSION "` + full + `"`,
			count:   1,
		},
		{
			path:    "build/windows/nsis/project.nsi",
			pattern: `(?m)^!define INFO_WINDOWS_VERSION "[^"]+"`,
			value:   `!define INFO_WINDOWS_VERSION "` + windows + `"`,
			count:   1,
		},
	}
	// 先验证全部配置，避免因配置格式不匹配只更新部分文件。
	outputs := make(map[string][]byte)
	for _, change := range changes {
		path := filepath.Join(root, change.path)
		data, ok := outputs[path]
		if !ok {
			data, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		pattern := regexp.MustCompile(change.pattern)
		matches := pattern.FindAllIndex(data, change.count)
		if len(matches) != change.count {
			return fmt.Errorf("版本配置不匹配: %s", path)
		}
		for i := len(matches) - 1; i >= 0; i-- {
			match := matches[i]
			updated := append([]byte{}, data[:match[0]]...)
			updated = append(updated, change.value...)
			updated = append(updated, data[match[1]:]...)
			data = updated
		}
		outputs[path] = data
	}
	for path, data := range outputs {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	fmt.Println("构建版本:", full)
	return nil
}
