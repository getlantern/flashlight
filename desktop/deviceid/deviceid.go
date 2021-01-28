package deviceid

import (
	"encoding/base64"

	"github.com/google/uuid"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("desktop.deviceid")
)

// OldStyleDeviceID returns the old style of device ID, which is derived from the MAC address.
func OldStyleDeviceID() string {
	return base64.StdEncoding.EncodeToString(uuid.NodeID())
}
