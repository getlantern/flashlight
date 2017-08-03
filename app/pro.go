package app

import (
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/ws"
)

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
