package app

import (
	"time"

	proClient "github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/golog"
)

// ProChecker is an interface for checking if a user is pro.
type ProChecker interface {
	IsProUser() (bool, bool)
}

type proChecker struct {
	settings *Settings
	log      golog.Logger
}

// newProChecker creates a new pro checker with the specified settings.
func newProChecker(settings *Settings) ProChecker {
	return &proChecker{settings: settings, log: golog.LoggerFor("flashlight.app.pro-checker")}
}

// IsProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func (pc *proChecker) IsProUser() (isPro bool, ok bool) {
	var userID int
	for {
		userID = int(pc.settings.GetUserID())
		if userID > 0 {
			break
		}
		pc.log.Debugf("Waiting for user ID to become non-zero")
		time.Sleep(10 * time.Second)
	}
	status, err := pc.userStatus(pc.settings.GetDeviceID(), userID, pc.settings.GetToken())
	if err != nil {
		pc.log.Errorf("Error getting user status? %v", err)
		return false, false
	}
	pc.log.Debugf("User %d is '%v'", userID, status)
	return status == "active", true
}

func (pc *proChecker) userStatus(deviceID string, userID int, proToken string) (string, error) {
	pc.log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
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
