package hellocap

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"

	"golang.org/x/sys/windows/registry"
)

var execPathRegexp = regexp.MustCompile(`"(.*)".*".*"`)

func defaultBrowser(ctx context.Context) (browser, error) {
	// TODO: test on Windows < 10 ?

	// https://stackoverflow.com/a/12444963?
	// TODO: may need https://stackoverflow.com/a/2178637 for older versions of Windows
	userChoice, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.html\UserChoice`, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser from registry: %w", err)
	}
	progID, _, err := userChoice.GetStringValue(`ProgId`)
	if err != nil {
		return nil, fmt.Errorf("failed to read browser program ID from registry: %w", err)
	}
	fmt.Println("progID:", progID)

	var appName string
	if progID == "htmlfile" {
		// The default browser is Internet Explorer and there is no application entry in the registry.
		appName = "Microsoft Internet Explorer"
	} else {
		application, err := registry.OpenKey(registry.CLASSES_ROOT, fmt.Sprintf(`%s\Application`, progID), registry.READ)
		if err != nil {
			return nil, fmt.Errorf("failed to read default browser application info from registry: %w", err)
		}
		appName, _, err = application.GetStringValue(`ApplicationName`)
		if err != nil {
			return nil, fmt.Errorf("failed to read default browser name from registry: %w", err)
		}
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

	switch appName {
	case "Microsoft Edge":
		// TODO: detect difference between Edge and EdgeHTML
		fmt.Println("default browser is Edge")

		matches := execPathRegexp.FindStringSubmatch(execPath)
		if len(matches) <= 1 {
			return nil, errors.New("unexpected executable path structure for Edge - Chromium")
		}
		fmt.Printf("using Edge with path '%s'\n", matches[1])
		return edgeChromium{matches[1]}, nil

	case "Microsoft Internet Explorer":
		// TODO: implement me!
		return nil, errors.New("unsupported browser - Internet Explorer")

	case "Google Chrome":
		fmt.Println("default browser is Chrome")

		// TODO: this is duplicated with Edge - abstract if this is a common pattern
		matches := execPathRegexp.FindStringSubmatch(execPath)
		if len(matches) <= 1 {
			return nil, errors.New("unexpected executable path structure for Chrome")
		}
		fmt.Printf("using Chrome with path '%s'\n", matches[1])
		return chrome{matches[1]}, nil

	default:
		return nil, fmt.Errorf("unsupported browser %s", appName)
	}
}

// TODO: the edge browsers may apply to other OSes as well

type edgeChromium struct {
	path string
}

func (ec edgeChromium) name() string {
	return "Microsoft Edge - Chromium"
}

func (ec edgeChromium) get(ctx context.Context, addr string) error {
	if err := exec.CommandContext(ctx, ec.path, "--headless", addr).Run(); err != nil {
		return fmt.Errorf("failed to execute binary: %w", err)
	}
	return nil
}

// EdgeHTML or Microsoft Edge Legacy is the older, HTML-based version of the Edge browser.
// https://support.microsoft.com/en-us/help/4026494/microsoft-edge-difference-between-legacy
type edgeHTML struct{}

func (eh edgeHTML) name() string {
	return "Microsoft Edge - HTML"
}

func (eh edgeHTML) get(ctx context.Context, addr string) error {
	// TODO: implement me!
	return errors.New("edge HTML is not supported")
}
