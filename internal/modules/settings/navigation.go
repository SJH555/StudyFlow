package settings

import (
	"errors"
	"strconv"
)

func validateNavigationWidths(in map[string]string) error {
	for key, bounds := range map[string]struct{ min, max int }{
		"primaryNavWidth": {72, 250},
		"sectionNavWidth": {72, 240},
	} {
		value, present := in[key]
		if !present {
			continue
		}
		width, err := strconv.Atoi(value)
		if err != nil || width < bounds.min || width > bounds.max {
			return errors.New("导航宽度超出允许范围")
		}
	}
	return nil
}
