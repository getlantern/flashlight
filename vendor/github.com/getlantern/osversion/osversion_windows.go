package osversion

import (
	"fmt"
	"os/exec"
	"regexp"
	"syscall"
)

var (
	verRegex = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)
)

func GetString() (string, error) {
	cmd := exec.Command("cmd", "ver")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	_text, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Unable to run ver: %v", err)
	}
	text := string(_text)
	if err != nil {
		return "", fmt.Errorf("Unable to run ver.exe: %v", err)
	}
	matches := verRegex.FindAllString(text, -1)
	if len(matches) != 1 {
		return "", fmt.Errorf("Version string not found: %v", text)
	}
	return matches[0], nil
}

func GetHumanReadable() (string, error) {
	version, err := GetSemanticVersion()
	if err != nil {
		return "", err
	}

	vstr := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)

	// Try to find the specific build first
	if str, ok := specificVersions[vstr]; ok {
		return str, nil
	}

	// Otherwise try with the generic list
	vstr = fmt.Sprintf("%d.%d", version.Major, version.Minor)
	if str, ok := versions[vstr]; ok {
		return str, nil
	} else {
		return "", fmt.Errorf("Unknown Windows version: %v", vstr)
	}
}

var specificVersions = map[string]string{
	"5.1.2505":   "Windows XP (RC 1)",
	"5.1.2600":   "Windows XP",
	"5.2.3541":   "Windows .NET Server interim",
	"5.2.3590":   "Windows .NET Server Beta 3",
	"5.2.3660":   "Windows .NET Server Release Candidate 1 (RC1)",
	"5.2.3718":   "Windows .NET Server 2003 RC2",
	"5.2.3763":   "Windows Server 2003",
	"5.2.3790":   "Windows Server 2003 / Windows Home Server",
	"6.0.5048":   "Windows Longhorn",
	"6.0.5112":   "Windows Vista, Beta 1",
	"6.0.5219":   "Windows Vista, CTP",
	"6.0.5259":   "Windows Vista, TAP Preview",
	"6.0.5270":   "Windows Vista, TAP Dec",
	"6.0.5308":   "Windows Vista, TAP Feb",
	"6.0.5342":   "Windows Vista, TAP Refresh",
	"6.0.5365":   "Windows Vista, April EWD",
	"6.0.5381":   "Windows Vista, Beta 2 Preview",
	"6.0.5384":   "Windows Vista, Beta 2",
	"6.0.5456":   "Windows Vista, Pre-RC1",
	"6.0.5472":   "Windows Vista, Pre-RC1, Build 5472",
	"6.0.5536":   "Windows Vista, Pre-RC1, Build 5536",
	"6.0.5600":   "Windows Vista, RC1",
	"6.0.5700":   "Windows Vista, Pre-RC2",
	"6.0.5728":   "Windows Vista, Pre-RC2, Build 5728",
	"6.0.5744":   "Windows Vista, RC2",
	"6.0.5808":   "Windows Vista, Pre-RTM, Build 5808",
	"6.0.5824":   "Windows Vista, Pre-RTM, Build 5824",
	"6.0.5840":   "Windows Vista, Pre-RTM, Build 5840",
	"6.0.6000":   "Windows Vista",
	"6.0.6002":   "Windows Vista, Service Pack 2",
	"6.0.6001":   "Windows Server 2008",
	"6.1.7600":   "Windows 7, RTM / Windows Server 2008 R2, RTM",
	"6.1.7601":   "Windows 7 / Windows Server 2008 R2, SP1",
	"6.1.8400":   "Windows Home Server 2011",
	"6.2.9200":   "Windows 8 / Windows Server 2012",
	"6.2.10211":  "Windows Phone 8",
	"6.3.9200":   "Windows 8.1 / Windows Server 2012 R2",
	"6.3.9600":   "Windows 8.1, Update 1",
	"10.0.10240": "Windows 10 RTM",
	"10.0.10586": "Windows 10 1511 / Windows Server 2016 Technical Preview 4",
	"10.0.14393": "Windows 10 1607 / Windows Server 2016",
	"10.0.15063": "Windows 10 1703",
	"6.4.9841":   "Windows Server 2016 Technical Preview",
	"10.0.10074": "Windows Server 2016 Technical Preview 2",
	"10.0.10514": "Windows Server 2016 Technical Preview 3",
	"10.0.14300": "Windows Server 2016 Technical Preview 5",
}

var versions = map[string]string{
	"5.0":  "Windows 2000 Professional / Windows 2000 Server (unknown build)",
	"5.1":  "Windows XP (unknown build)",
	"5.2":  "Windows XP Professional x64 / Windows Server 2003 (unknown build)",
	"6.0":  "Windows Vista / Windows Server 2008 (unknown build)",
	"6.1":  "Windows 7 / Windows Server 2008 R2 (unknown build)",
	"6.2":  "Windows 8 / Windows Server 2012 (unknown build)",
	"6.3":  "Windows 8.1 / Windows Server 2012 R2 (unknown build)",
	"10.0": "Windows 10 / Windows Server 2016 (unknown build)",
}
