//go:build !android

package osversion

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/host"
)

func GetHumanReadable() (string, error) {
	a, err := host.Info()
	if err != nil {
		return "", fmt.Errorf("failed to get host info: %w", err)
	}
	return fmt.Sprintf("%s %s", a.Platform, a.PlatformVersion), nil
}

func GetSemanticVersion() (string, error) {
	a, err := host.Info()
	if err != nil {
		return "", fmt.Errorf("failed to get host info: %w", err)
	}
	return a.PlatformVersion, nil
}
