package client

import (
	"github.com/getlantern/flashlight/pro"
)

// IsProUser indicates whether or not the user is pro, calling the Pro API if
// necessary to determine the status.
func IsProUser(userID int, proToken string, deviceID string) (isPro bool, statusKnown bool) {
	isPro, statusKnown = pro.IsProUserFast(userID)
	if statusKnown {
		return
	}
	status, err := userStatus(userID, proToken, deviceID)
	if err != nil {
		log.Errorf("Error getting user status? %v", err)
		return false, false
	}
	log.Debugf("User %d is '%v'", userID, status)
	pro.SetProStatus(userID, status)
	return pro.IsActive(status), true
}

func userStatus(userID int, proToken string, deviceID string) (string, error) {
	log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user := User{Auth: Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	req, err := NewRequest(user)
	if err != nil {
		return "", err
	}
	resp, err := UserStatus(req)
	if err != nil {
		return "", err
	}
	return resp.User.UserStatus, nil
}
