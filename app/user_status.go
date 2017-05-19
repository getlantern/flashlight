package app

import (
	proClient "github.com/getlantern/flashlight/pro/client"
)

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
