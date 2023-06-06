package common

const (
	// DevLibraryVersion specifies what version of the library to report when in development
	DevLibraryVersion = "9999.99.99"

	// DefaultApplicationVersion is the default version of the application for auto-update
	// purposes. while in development mode we probably would not want auto-updates to be
	// applied. Using a big number here prevents such auto-updates without
	// disabling the feature completely. The "make package-*" tool will take care
	// of bumping this version number so you don't have to do it by hand.
	DefaultApplicationVersion = "9999.99.99-dev"
)

var (
	// CompileTimeApplicationVersion is set at compile-time by application production builds
	CompileTimeApplicationVersion string = ""

	// ApplicationVersion is the version of the package to use depending on if we're
	// in development, production, etc. ApplicationVersion is used by the Features mechanism
	// to determine which features to enable/disable.
	ApplicationVersion = bestApplicationVersion()

	// LibraryVersion is hardcoded. LibraryVersion is mostly used in the X-Lantern-Version header
	// for purposes of proxy assignment.
	LibraryVersion = "7.5.0"

	// RevisionDate is the date of the most recent code revision.
	RevisionDate string // The revision date and time that is associated with the version string.

	// BuildDate is the date the code was actually built.
	BuildDate string // The actual date and time the binary was built.
)

func bestApplicationVersion() string {
	if CompileTimeApplicationVersion != "" {
		return CompileTimeApplicationVersion
	}
	return DefaultApplicationVersion
}

func init() {
	if InDevelopment() {
		LibraryVersion = DevLibraryVersion
	}

	if RevisionDate == "" {
		RevisionDate = "now"
	}
}

// InDevelopment indicates whether this built was built in development.
func InDevelopment() bool {
	return ApplicationVersion == DefaultApplicationVersion
}
