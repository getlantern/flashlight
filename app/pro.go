package app

import (
	"time"

	"github.com/getlantern/flashlight/pro"
)

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func isProUser() (isPro bool, ok bool) {
	var userID int64
	for {
		userID = settings.GetUserID()
		if userID > 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return pro.IsProUser(userID, settings.GetToken(), settings.GetDeviceID())
}

// isProUserFast checks a cached value for the pro status and doesn't wait for
// an answer. It assumes that isProUser is called somewhere along the line in
// order to update the status.
func isProUserFast() (isPro bool, statusKnown bool) {
	userID := settings.GetUserID()
	if userID == 0 {
		return false, false
	}
	return pro.IsProUserFast(userID)
}

// func servePro() error {
// 	helloFn := func(write func(interface{})) {
// 		isPro, statusKnown := isProUserFast()
// 		log.Debugf("Sending current user data to new client: %v", theSession)
// 		write(theSession)
// 	}
// 	service, err := ws.Register("pro", helloFn)
// 	if err != nil {
// 		log.Errorf("Error registering with UI? %v", err)
// 		return err
// 	}
// 	go func() {
// 		for _ = range theSession.signal {
// 			service.Out <- theSession.copy()
// 		}
// 	}()
// }
