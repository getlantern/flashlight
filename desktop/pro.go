package desktop

import (
	"time"

	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"
)

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func isProUser() (isPro bool, ok bool) {
	_, err := settings.GetInt64Eventually(SNUserID)
	if err != nil {
		return false, false
	}
	return pro.IsProUser(settings)
}

// isProUserFast checks a cached value for the pro status and doesn't wait for
// an answer. It works because servePro below fetches user data / create new
// user when starts up. The pro proxy also updates user data implicitly for
// '/userData' calls initiated from desktop UI.
func isProUserFast() (isPro bool, statusKnown bool) {
	return pro.IsProUserFast(settings)
}

// servePro fetches user data or creates new user when the application starts
// up or a new WebSocket client is connected, and serves user data to all
// connected WebSocket clients via the "pro" channel.
// It loops forever in 10 seconds interval until the user is fetched or
// created, as it's fundamental for the UI to work.
func servePro(channel ws.UIChannel) error {
	logger := golog.LoggerFor("flashlight.app.pro")
	chFetch := make(chan bool)
	retry := time.NewTimer(0)
	retryLater := func() {
		if !retry.Stop() {
			<-retry.C
		}
		retry.Reset(10 * time.Second)
	}

	fetchOrCreate := func() {
		userID := settings.GetUserID()
		if userID == "" {
			user, err := pro.NewUser(settings)
			if err != nil {
				logger.Errorf("Could not create new Pro user: %v", err)
				retryLater()
			} else {
				settings.SetUserIDAndToken(user.Auth.ID, user.Auth.Token)
			}
		} else {
			_, err := pro.FetchUserData(settings)
			if err != nil {
				logger.Errorf("Could not get user data for %v: %v", userID, err)
				retryLater()
			}
		}
	}
	go func() {
		for {
			select {
			case <-chFetch:
				fetchOrCreate()
			case <-retry.C:
				fetchOrCreate()
			}
		}
	}()

	helloFn := func(write func(interface{})) {
		if user, known := pro.GetUserDataFast(settings.GetUserID()); known {
			logger.Debugf("Sending current user data to new client: %v", user)
			write(user)
		}
		logger.Debugf("Fetching user data again to see if any changes")
		select {
		case chFetch <- true:
		default: // fetching in progress, skipping
		}
	}
	service, err := channel.Register("pro", helloFn)
	pro.OnUserData(func(current *client.User, new *client.User) {
		logger.Debugf("Sending updated user data to all clients: %v", new)
		service.Out <- new
	})

	return err
}
