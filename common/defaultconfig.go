// +build !lantern
// +build !beam

package common

const (
	// AppName is the name of this particular app, such as Lantern or Beam
	AppName = "Default"

	// ProAvailable specifies whether the user can purchase pro with this version.
	ProAvailable = false

	// TrackingID is the Google Analytics tracking ID.
	TrackingID = "UA-21815217-23"

	// SentryDSN is Sentry's project ID thing
	SentryDSN = "https://f65aa492b9524df79b05333a0b0924c5@sentry.io/2222244"

	// UpdateServerURL is the URL of the update server.
	UpdateServerURL = "https://update.getlantern.org/update/getlantern/lantern"
)
