package common

import (
	"os"
	"slices"
	"strings"
)

const (
	// The default name for this app (used if no client-supplied name is passed at initialization)
	DefaultAppName = "Lantern"

	// ProAvailable specifies whether the user can purchase pro with this version.
	ProAvailable = true

	// TrackingID is the Google Analytics tracking ID.
	TrackingID = "UA-21815217-12"
)

var transports = loadTransports()

func loadTransports() []string {
	env := os.Getenv("LANTERN_TRANSPORTS")
	if env == "" {
		return []string{}
	}
	parts := strings.Split(env, ",")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

// SupportsTransport reads the LANTERN_TRANSPORTS environment variable and returns whether or not the
// specified transport is supported. If there is no LANTERN_TRANSPORTS environment variable defined,
// all transports are supported.
func SupportsTransport(transport string) bool {
	return len(transports) == 0 || slices.Contains(transports, transport)
}
