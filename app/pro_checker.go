package app

import (
	"sync"
	"time"

	"github.com/getlantern/flashlight/common"
	proClient "github.com/getlantern/flashlight/pro/client"
)

var configureProClientOnce sync.Once

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func isProUser() (isPro bool, ok bool) {
	configureProClientOnce.Do(func() {
		proClient.Configure(stagingMode, common.PackageVersion)
	})

	var userID int
	for {
		userID = int(settings.GetUserID())
		if userID > 0 {
			break
		}
		log.Debugf("Waiting for user ID to become non-zero")
		time.Sleep(10 * time.Second)
	}
	status, err := userStatus(settings.GetDeviceID(), userID, settings.GetToken())
	if err != nil {
		log.Errorf("Error getting user status? %v", err)
		return false, false
	}
	log.Debugf("User %d is '%v'", userID, status)
	return status == "active", true
}

func userStatus(deviceID string, userID int, proToken string) (string, error) {
	log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user := proClient.User{Auth: proClient.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	req, err := proClient.NewRequest(user)
	if err != nil {
		return "", err
	}
	resp, err := proClient.UserStatus(req)
	if err != nil {
		return "", err
	}
	return resp.User.UserStatus, nil
}
