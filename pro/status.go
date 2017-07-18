package pro

import (
	"sync"

	"github.com/getlantern/flashlight/pro/client"
)

var (
	proStatusByUserID = make(map[int64]string)
	proStatusMx       sync.RWMutex
)

// SetProStatus updates the pro status for the given userID.
func SetProStatus(userID int64, status string) {
	proStatusMx.Lock()
	proStatusByUserID[userID] = status
	proStatusMx.Unlock()
}

// IsProUserFast indicates whether or not the user is pro and whether or not the
// user's status is know, never calling the Pro API to determine the status.
func IsProUserFast(userID int64) (isPro bool, statusKnown bool) {
	proStatusMx.RLock()
	status, found := proStatusByUserID[userID]
	proStatusMx.RUnlock()
	return IsActive(status), found
}

// IsActive determines whether the given status is an active status
func IsActive(status string) bool {
	return status == "active"
}

// IsProUser indicates whether or not the user is pro, calling the Pro API if
// necessary to determine the status.
func IsProUser(userID int64, proToken string, deviceID string) (isPro bool, statusKnown bool) {
	isPro, statusKnown = IsProUserFast(userID)
	if statusKnown {
		return
	}
	status, err := userStatus(userID, proToken, deviceID)
	if err != nil {
		log.Errorf("Error getting user status? %v", err)
		return false, false
	}
	log.Debugf("User %d is '%v'", userID, status)
	SetProStatus(userID, status)
	return IsActive(status), true
}

func userStatus(userID int64, proToken string, deviceID string) (string, error) {
	log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user := client.User{Auth: client.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	cli := client.NewClient(GetHTTPClient())
	resp, err := cli.UserStatus(user)
	if err != nil {
		return "", err
	}
	return resp.User.UserStatus, nil
}
