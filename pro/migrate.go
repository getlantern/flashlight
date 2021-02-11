package pro

import (
	"net/http"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
)

// MigrateDeviceID migrates the user's device ID from the old to the new scheme (only relevant to desktop builds)
func MigrateDeviceID(uc common.UserConfig, oldDeviceID string) error {
	return migrateDeviceIDWithClient(uc, oldDeviceID, httpClient)
}

func migrateDeviceIDWithClient(uc common.UserConfig, oldDeviceID string, hc *http.Client) error {
	logger.Debugf("Migrating deviceID from %v to %v", oldDeviceID, uc.GetDeviceID())
	return client.NewClient(hc, PrepareProRequestWithOptions).MigrateDeviceID(uc, oldDeviceID)
}
