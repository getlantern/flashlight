package hellocap

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

var execPathRegexp = regexp.MustCompile(`"(.*)".*".*"`)

func defaultBrowser(ctx context.Context) (browser, error) {
	// TODO: test on Windows < 10 ?

	// https://stackoverflow.com/a/2178637
	// TODO: or maybe https://stackoverflow.com/a/12444963?
	// k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\http\shell\open\command\(Default)`, registry.READ)
	userChoice, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.html\UserChoice`, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser from registry: %w", err)
	}

	// debugging
	// names, err := k.ReadSubKeyNames(100)
	// if err != nil {
	// 	fmt.Println("failed to read subkey names:", err)
	// } else {
	// 	fmt.Println("read", len(names), "subkey names:")
	// 	for _, name := range names {
	// 		fmt.Println(name)
	// 	}
	// 	fmt.Println()
	// }
	// names, err = k.ReadValueNames(100)
	// if err != nil {
	// 	fmt.Println("failed to read value names:", err)
	// } else {
	// 	fmt.Println("read", len(names), "value names:")
	// 	for _, name := range names {
	// 		fmt.Println(name)
	// 	}
	// 	fmt.Println()
	// }

	progID, _, err := userChoice.GetStringValue(`ProgId`)
	if err != nil {
		return nil, fmt.Errorf("failed to read browser program ID from registry: %w", err)
	}
	fmt.Println("progID:", progID)
	application, err := registry.OpenKey(registry.CLASSES_ROOT, fmt.Sprintf(`%s\Application`, progID), registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser application info from registry: %w", err)
	}
	appName, _, err := application.GetStringValue(`ApplicationName`)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser name from registry: %w", err)
	}
	fmt.Println("appName:", appName)
	appExec, err := registry.OpenKey(registry.CLASSES_ROOT, fmt.Sprintf(`%s\Shell\open\command`, progID), registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser executable info from registry: %w", err)
	}
	execPath, _, err := appExec.GetStringValue("")
	if err != nil {
		return nil, fmt.Errorf("failed to read path to default browser executable from registry: %w", err)
	}
	fmt.Println("execPath:", execPath)

	switch {
	case strings.Contains(appName, "MicrosoftEdge"):
		fmt.Println("default browser is Edge")
	case appName == "Google Chrome":
		fmt.Println("default browser is Chrome")

		matches := execPathRegexp.FindStringSubmatch(execPath)
		if len(matches) <= 1 {
			return nil, errors.New("unexpected executable path structure for Chrome")
		}
		fmt.Printf("using Chrome with path '%s'\n", matches[1])
		return chrome{matches[1]}, nil
	}

	switch appName {
	// TODO: figure out key for IE and Edge
	case "IE.AssocFile.HTM": // TODO: .HTML?
		// TODO: implement me!
		return nil, nil
	case "FirefoxHTML":
		// TODO: implement me!
		return nil, nil
	case "ChromeHTML":
		// TODO: implement me!
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported browser %s", appName)
	}
}
