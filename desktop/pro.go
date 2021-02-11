package desktop

import (
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/desktop/deviceid"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/flashlight/ws"
)

// isProUser blocks itself to check if current user is Pro, or !ok if error
// happens getting user status from pro-server. The result is not cached
// because the user can become Pro or free at any time. It waits until
// the user ID becomes non-zero.
func isProUser() (isPro bool, ok bool) {
	settings := getSettings()
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
	return pro.IsProUserFast(getSettings())
}

// servePro fetches user data or creates new user when the application starts
// up or a new WebSocket client is connected, and serves user data to all
// connected WebSocket clients via the "pro" channel.
// It loops forever in 10 seconds interval until the user is fetched or
// created, as it's fundamental for the UI to work.
func servePro(channel ws.UIChannel) error {
	logger := golog.LoggerFor("flashlight.app.pro")
	settings := getSettings()
	chFetch := make(chan bool)
	go func() {
		fetchOrCreate := func() error {
			userID := settings.GetUserID()
			if userID == 0 {
				user, err := pro.NewUser(settings)
				if err != nil {
					return errors.New("Could not create new Pro user: %v", err)
				}
				settings.SetUserIDAndToken(user.Auth.ID, user.Auth.Token)
			} else {
				isPro, _ := pro.IsProUserFast(settings)
				if isPro && userID != settings.GetMigratedDeviceIDForUserID() {
					// If we've gotten here, that means this client may have previously used an old-style device ID. We don't know for sure,
					// because it's possible that the user never used an old version of Lantern on this device. In either case, it's safe
					// to request to migrate the device ID, as the server will know whether or not the old-style device ID was already associated
					// with the current pro user.
					oldStyleDeviceID := deviceid.OldStyleDeviceID()
					if oldStyleDeviceID != settings.GetDeviceID() {
						log.Debugf("Attempting to migrate device ID from %v to %v", oldStyleDeviceID, settings.GetDeviceID())
						err := pro.MigrateDeviceID(settings, oldStyleDeviceID)
						if err != nil {
							errString := err.Error()
							if errString == "old_device_id_not_found" {
								log.Debugf("Could not migrate device id, not fatal: %v", err)
							} else {
								return log.Errorf("Could not migrate device id: %v", err)
							}
						} else {
							log.Debug("Successfully migrated device ID")
						}
						settings.SetMigratedDeviceIDForUserID(userID)
					}
				}
				_, err := pro.FetchUserData(settings)
				if err != nil {
					return errors.New("Could not get user data for %v: %v", userID, err)
				}
			}
			return nil
		}

		retry := time.NewTimer(0)
		retryOnFail := func(drainChannel bool) {
			if err := fetchOrCreate(); err != nil {
				if drainChannel && !retry.Stop() {
					<-retry.C
				}
				retry.Reset(10 * time.Second)
			}
		}
		for {
			select {
			case <-chFetch:
				retryOnFail(true)
			case <-retry.C:
				retryOnFail(false)
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
