package common

import (
	"runtime/debug"
	"strings"

	"github.com/blang/semver"
)

var (
	// CompileTimeApplicationVersion is set at compile-time by application production builds
	CompileTimeApplicationVersion string = ""

	// LibraryVersion is determined at runtime based on the version of the lantern library that's been included.
	LibraryVersion = ""

	// BuildDate is the date the code was actually built.
	BuildDate string // The actual date and time the binary was built.
)

func init() {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		panic("Unable to read build info")
	}

versionLoop:
	for _, dep := range buildInfo.Deps {
		if strings.HasPrefix(dep.Path, "github.com/getlantern/flashlight/v7") && strings.HasPrefix(dep.Version, "v") {
			version := dep.Version[1:]
			log.Debugf("Flashlight version is %v", version)
			_, parseErr := semver.Parse(version)
			if parseErr == nil {
				log.Debugf("Setting LibraryVersion to %v", version)
				LibraryVersion = version
				break versionLoop
			}
		}
	}
}
