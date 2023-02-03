package osversion

import (
	"fmt"
	"github.com/shirou/gopsutil/v3/host"
	"os/exec"
	"strconv"
	"strings"
)

func getAndroidAPI() (int, error) {
	out, err := exec.Command("getprop", "ro.build.version.sdk").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get Android API version: %w", err)
	}

	return strconv.Atoi(strings.TrimSpace(string(out)))
}

func getAndroidOSVersion() (int, error) {
	out, err := exec.Command("getprop", "ro.build.version.release").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get Android OS version: %w", err)
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

func GetHumanReadable() (string, error) {
	a, err := host.Info()
	if err != nil {
		return "", fmt.Errorf("failed to get host info: %w", err)
	}

	// Just ignore the errors. If we fail to get those values, we just don't
	// include it.
	apiVersion, _ := getAndroidAPI()
	if err != nil {
		apiVersion = 0
	}
	release, _ := getAndroidOSVersion()
	if err != nil {
		release = 0
	}

	return fmt.Sprintf("Android (API %d | OS %d | Arch %s)",
		apiVersion, release, a.KernelArch), nil
}

func GetSemanticVersion() (string, error) {
	release, err := getAndroidOSVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get Android OS version: %w", err)
	}
	return fmt.Sprintf("%d", release), nil
}
