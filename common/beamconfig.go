// +build beam

package common

const (
	// AppName is the name of this specific app.
	AppName = "Beam"

	// ProAvailable specifies whether the user can purchase pro with this version.
	ProAvailable = false

	// TrackingID is the Google Analytics tracking ID.
	TrackingID = "UA-21815217-23"
)

var (
	// GlobalURL URL for fetching the global config.
	GlobalURL = "https://globalconfig.flashlightproxy.com/beamglobal.yaml.gz"
)
