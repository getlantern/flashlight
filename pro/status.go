package pro

import (
	"sync"

	"github.com/getlantern/flashlight/pro/client"
)

var (
	userDataByUserID = make(map[int64]client.User)
	userDataMx       sync.RWMutex
)

// SetUserData updates the user data for the given userID.
func SetUserData(userID int64, user client.User) {
	userDataMx.Lock()
	userDataByUserID[userID] = user
	userDataMx.Unlock()
}

// IsProUser indicates whether or not the user is pro, calling the Pro API if
// necessary to determine the status.
func IsProUser(userID int64, proToken string, deviceID string) (isPro bool, statusKnown bool) {
	isPro, statusKnown = IsProUserFast(userID)
	if statusKnown {
		return
	}
	user, err := userData(userID, proToken, deviceID)
	if err != nil {
		log.Errorf("Error getting user status: %v", err)
		return false, false
	}
	SetUserData(userID, *user)
	log.Debugf("User %d is '%v'", userID, user.UserStatus)
	return IsActive(user.UserStatus), true
}

// IsProUserFast indicates whether or not the user is pro and whether or not the
// user's status is know, never calling the Pro API to determine the status.
func IsProUserFast(userID int64) (isPro bool, statusKnown bool) {
	userDataMx.RLock()
	user, found := userDataByUserID[userID]
	userDataMx.RUnlock()
	return IsActive(user.UserStatus), found
}

// IsActive determines whether the given status is an active status
func IsActive(status string) bool {
	return status == "active"
}

func userData(userID int64, proToken string, deviceID string) (*client.User, error) {
	log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user := client.User{Auth: client.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	resp, err := client.NewClient(GetHTTPClient()).UserStatus(user)
	if err != nil {
		return nil, err
	}
	return &resp.User, nil
}
