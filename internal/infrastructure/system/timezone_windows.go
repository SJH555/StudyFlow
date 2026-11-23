package system

import "golang.org/x/sys/windows/registry"

func SystemTimeZone() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\TimeZoneInformation`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()
	id, _, err := key.GetStringValue("TimeZoneKeyName")
	return id, err
}
