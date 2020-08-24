package simbrowser

import (
	"context"
	"fmt"

	"github.com/getlantern/flashlight/browsers"
)

// mimicDefaultBrowser chooses a Browser which mimics the system's default web browser.
func mimicDefaultBrowser(ctx context.Context) (Browser, error) {
	b, err := browsers.SystemDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain default browser: %w", err)
	}
	switch b {
	case browsers.Chrome:
		return chrome, nil
	case browsers.Firefox:
		return firefox, nil
	case browsers.Edge, browsers.EdgeLegacy:
		return edge, nil
	case browsers.InternetExplorer:
		return explorer, nil
	case browsers.ThreeSixtySecureBrowser:
		return threeSixty, nil
	case browsers.QQBrowser:
		return qq, nil
	default:
		return nil, fmt.Errorf("unsupported browser %v", b)
	}
}
