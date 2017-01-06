package app

import (
	"sync"

	"github.com/getlantern/errors"
	proClient "github.com/getlantern/pro-server-client/go-client"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/proxied"
)

var configureProClientOnce sync.Once

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached.
func isProUser() (isPro bool, ok bool) {
	configureProClientOnce.Do(func() {
		proClient.Configure(stagingMode, flashlight.PackageVersion)
	})

	userID := int(settings.GetUserID())
	status, err := userStatus(settings.GetDeviceID(), userID, settings.GetToken())
	if err != nil {
		log.Errorf("Error getting user status? %v", err)
		return false, false
	}
	log.Debugf("User %d is %v", userID, status)
	return status == "active", true
}

func userStatus(deviceID string, userID int, proToken string) (string, error) {
	log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user := proClient.User{Auth: proClient.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	http, err := proxied.GetHTTPClient(true)
	if err != nil {
		return "", errors.Wrap(err)
	}
	client := proClient.NewClient(http)
	resp, err := client.UserData(user)
	if err != nil {
		return "", errors.Wrap(err)
	}
	if resp.Status == "error" {
		return "", errors.New(resp.Error)
	}
	return resp.User.UserStatus, nil
}
