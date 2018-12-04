package common

const (
	// DefaultPackageVersion is the default version of the package for auto-update
	// purposes. while in development mode we probably would not want auto-updates to be
	// applied. Using a big number here prevents such auto-updates without
	// disabling the feature completely. The "make package-*" tool will take care
	// of bumping this version number so you don't have to do it by hand.
	DefaultPackageVersion = "9999.99.99"
)

var (
	// CompileTimePackageVersion is set at compile-time for production builds
	CompileTimePackageVersion string = ""

	// PackageVersion is the version of the package to use depending on if we're
	// in development, production, etc.
	PackageVersion = bestPackageVersion()

	// Version is the version of Lantern we're running.
	Version string

	// RevisionDate is the date of the most recent code revision.
	RevisionDate string // The revision date and time that is associated with the version string.

	// BuildDate is the date the code was actually built.
	BuildDate string // The actual date and time the binary was built.
)

func bestPackageVersion() string {
	if CompileTimePackageVersion != "" {
		return CompileTimePackageVersion
	}
	return DefaultPackageVersion
}

func init() {
	if PackageVersion != DefaultPackageVersion {
		// packageVersion has precedence over GIT revision. This will happen when
		// packing a version intended for release.
		Version = PackageVersion
	}

	if Version == "" {
		Version = DefaultPackageVersion + "-dev"
	}

	if RevisionDate == "" {
		RevisionDate = "now"
	}
}
