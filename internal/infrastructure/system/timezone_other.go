//go:build !windows && !linux

package system

import (
	"errors"
	"time"
)

func SystemTimeZone() (string, error) {
	zone := time.Local.String()
	if zone == "Local" {
		return "", errors.New("无法确认系统时区")
	}
	return zone, nil
}
