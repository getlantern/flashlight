package app

import (
	"time"

	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"
)

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func isProUser() (isPro bool, ok bool) {
	uid, err := settings.GetInt64Eventually(SNUserID)
	if err != nil {
		return false, false
	}
	return pro.IsProUser(uid, settings.GetToken(), settings.GetDeviceID())
}

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
// It loops forever in 10 seconds interval until the user is fetched or
// created, as it's fundamental for the UI to work.
func servePro() error {
	logger := golog.LoggerFor("flashlight.app.pro")
	go func() {
		userID := settings.GetUserID()
		for {
			if userID == 0 {
				user, err := pro.NewUser(settings.GetDeviceID())
				if err != nil {
					logger.Errorf("Could not create new Pro user: %v", err)
				} else {
					settings.SetUserID(user.Auth.ID)
					settings.SetToken(user.Auth.Token)
					return
				}
			} else {
				_, err := pro.GetUserData(userID, settings.GetToken(), settings.GetDeviceID())
				if err != nil {
					logger.Errorf("Could not get user data for %v: %v", userID, err)
				} else {
					return
				}
			}
			time.Sleep(10 * time.Second)
		}
	}()
	helloFn := func(write func(interface{})) {
		go func() {
			user := pro.WaitForUserData(settings.GetUserID())
			logger.Debugf("Sending current user data to new client: %v", user)
			write(user)
		}()
	}
	_, err := ws.Register("pro", helloFn)
	return err
}
