package app

import (
	"time"

	"github.com/getlantern/flashlight/pro"
	proclient "github.com/getlantern/flashlight/pro/client"
)

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func isProUser() (isPro bool, ok bool) {
	var userID int
	for {
		userID = int(settings.GetUserID())
		if userID > 0 {
			break
		}
		log.Debugf("Waiting for user ID to become non-zero")
		time.Sleep(10 * time.Second)
	}
	return proclient.IsProUser(userID, settings.GetToken(), settings.GetDeviceID())
}

// isProUserFast checks a cached value for the pro status and doesn't wait for
// an answer. It assumes that isProUser is called somewhere along the line in
// order to update the status.
func isProUserFast() (isPro bool, statusKnown bool) {
	userID := int(settings.GetUserID())
	if userID == 0 {
		return false, false
	}
	return pro.IsProUserFast(userID)
}
