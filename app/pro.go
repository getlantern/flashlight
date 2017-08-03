package app

import (
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/ws"
)

// isProUserFast checks a cached value for the pro status and doesn't wait for
// an answer. It works because servePro below fetches user data / create new
// user when starts up. The pro proxy also updates user data implicitly for
// '/userData' calls initiated from desktop UI.
func isProUserFast() (isPro bool, statusKnown bool) {
	userID := settings.GetUserID()
	if userID == 0 {
		return false, false
	}
	return pro.IsProUserFast(userID)
}

// servePro fetches user data or creates new user, and serves user data to all
// connected WebSocket clients via the "pro" channel.
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
			} else {
				_, err := pro.GetUserData(userID, settings.GetToken(), settings.GetDeviceID())
				if err != nil {
					log.Errorf("Could not get user data for %v: %v", userID, err)
				} else {
					return
				}
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
