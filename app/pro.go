package app

import (
	"time"

	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/ws"
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

func servePro() error {
	go func() {
		for {
			userID := settings.GetUserID()
			if userID == 0 {
				user, err := pro.NewUser(settings.GetDeviceID())
				if err != nil {
					log.Errorf("Could not create new Pro user: %v", err)
				} else {
					settings.SetUserID(user.Auth.ID)
					return
				}
			}
			_, err := pro.GetUserData(userID, settings.GetToken(), settings.GetDeviceID())
			if err != nil {
				log.Errorf("Could not get user data for %v: %v", userID, err)
			} else {
				return
			}
		}
	}()
	helloFn := func(write func(interface{})) {
		go func() {
			user := pro.WaitForUserData(settings.GetUserID())
			log.Debugf("Sending current user data to new client: %v", user)
			write(user)
		}()
	}
	_, err := ws.Register("pro", helloFn)
	return err
}
