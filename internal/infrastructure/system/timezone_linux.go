package system

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"exam_notify/internal/shared/studycalendar"
)

func SystemTimeZone() (string, error) {
	system, err := zoneFileName("/etc/localtime")
	if err != nil || !studycalendar.IsSupported(system) {
		return system, err
	}
	// 环境变量若覆盖 WebView 的墙上时间，也必须通过检查。

	if zone, set := os.LookupEnv("TZ"); set {
		zone = strings.TrimPrefix(zone, ":")
		if zone == "" {
			return "UTC", nil
		}
		if !filepath.IsAbs(zone) {
			return zone, nil
		}
		return zoneFileName(zone)
	}
	return system, nil
}

func zoneFileName(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if _, zone, found := strings.Cut(resolved, "/zoneinfo/"); found {
		return strings.TrimPrefix(strings.TrimPrefix(zone, "posix/"), "right/"), nil
	}
	// 部分发行版用文件副本配置 /etc/localtime。
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	china, err := os.ReadFile("/usr/share/zoneinfo/Asia/Shanghai")
	if err == nil && bytes.Equal(data, china) {
		return "Asia/Shanghai", nil
	}
	return "", err
}
