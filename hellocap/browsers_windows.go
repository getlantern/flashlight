package hellocap

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

func defaultBrowser() (browser, error) {
	// https://stackoverflow.com/a/2178637
	// TODO: or maybe https://stackoverflow.com/a/12444963?
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\http\shell\open\command`)
	if err != nil {
		return nil, fmt.Errorf("failed to read default browser from registry")
	}
	// TODO: really not sure about this
	// TODO: rename kString
	kString, _, err := k.GetStringValue("Default")
	switch kString {
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
		return nil, fmt.Errorf("unsupported browser %s", kString)
	}

	// TODO: implement me!
	return nil, nil
}
