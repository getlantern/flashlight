package ios

import (
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/fronted"

	"github.com/getlantern/flashlight/common"
	proclient "github.com/getlantern/flashlight/pro/client"
)

// ProCredentials are credentials that authenticate a pro account
type ProCredentials struct {
	UserID   int
	ProToken string
}

// IsActiveProDevice checks whether the given device is an active pro device
func IsActiveProDevice(userID int, proToken, deviceID string) (bool, error) {
	pc, err := getProClient()
	if err != nil {
		return false, err
	}

	resp, err := pc.UserData(userConfigFor(userID, proToken, deviceID))
	if err != nil {
		if strings.Contains(err.Error(), "Not authorized") {
			// This means that the user_id and pro_token are no good, which means this can't be an active pro device
			return false, nil
		}
		return false, log.Errorf("unable to fetch user data: %v", err)
	}

	user := resp.User
	if user.UserStatus != "active" {
		log.Debug("pro account not active")
		return false, nil
	}

	for _, device := range user.Devices {
		if device.Id == deviceID {
			return true, nil
		}
	}

	log.Debug("device is not linked to pro account")
	return false, nil
}

// RecoverProAccount attempts to recover an existing Pro account linked to this email address and device ID
func RecoverProAccount(deviceID, emailAddress string) (*ProCredentials, error) {
	pc, err := getProClient()
	if err != nil {
		return nil, err
	}

	resp, err := pc.RecoverProAccount(partialUserConfigFor(deviceID), emailAddress)
	if err != nil {
		return nil, log.Errorf("unable to recover pro account: %v", err)
	}

	return &ProCredentials{UserID: resp.UserID, ProToken: resp.ProToken}, nil
}

// RequestRecoveryEmail requests an account recovery email for linking to an existing pro account
func RequestRecoveryEmail(deviceID, deviceName, emailAddress string) error {
	pc, err := getProClient()
	if err != nil {
		return err
	}

	err = pc.RequestRecoveryEmail(partialUserConfigFor(deviceID), deviceName, emailAddress)
	if err != nil {
		return log.Errorf("unable to request recovery email: %v", err)
	}

	return nil
}

// ValidateRecoveryCode validates the given recovery code and finishes linking the device, returning the user_id and pro_token for the account.
func ValidateRecoveryCode(deviceID, code string) (*ProCredentials, error) {
	pc, err := getProClient()
	if err != nil {
		return nil, err
	}

	resp, err := pc.ValidateRecoveryCode(partialUserConfigFor(deviceID), code)
	if err != nil {
		return nil, log.Errorf("unable to validate recovery code: %v", err)
	}

	return &ProCredentials{UserID: resp.UserID, ProToken: resp.ProToken}, nil
}

// RequestDeviceLinkingCode requests a new device linking code to allow linking the current device to a pro account via an existing pro device.
func RequestDeviceLinkingCode(deviceID, deviceName string) (string, error) {
	pc, err := getProClient()
	if err != nil {
		return "", err
	}

	resp, err := pc.RequestDeviceLinkingCode(partialUserConfigFor(deviceID), deviceName)
	if err != nil {
		return "", log.Errorf("unable to request link code: %v", err)
	}

	return resp.Code, nil
}

// Canceler providers a mechanism for canceling long running operations
type Canceler struct {
	c chan interface{}
}

// Cancel cancels an operation
func (c *Canceler) Cancel() {
	select {
	case c.c <- nil:
		// submitted
	default:
		// nothing to cancel
	}
}

// NewCanceler creates a Canceller
func NewCanceler() *Canceler {
	return &Canceler{c: make(chan interface{})}
}

// ValidateDeviceLinkingCode validates a device linking code to allow linking the current device to a pro account via an existing pro device.
// It will keep trying until it succeeds or the supplied Canceler is canceled. In the case of cancel, it will return nil credentials and a nil
// error.
func ValidateDeviceLinkingCode(c *Canceler, deviceID, deviceName, code string) (*ProCredentials, error) {
	pc, err := getProClient()
	if err != nil {
		return nil, err
	}

	overallTimeout := time.After(5 * time.Minute)
	retryDelay := 5 * time.Second
	maxRetryDelay := 30 * time.Second
	for {
		resp, err := pc.ValidateDeviceLinkingCode(partialUserConfigFor(deviceID), deviceName, code)
		if err == nil {
			return &ProCredentials{UserID: resp.UserID, ProToken: resp.ProToken}, nil
		}

		err = log.Errorf("unable to validate recovery code: %v", err)
		select {
		case <-time.After(retryDelay):
			retryDelay *= 2
			if retryDelay > maxRetryDelay {
				retryDelay = maxRetryDelay
			}
			log.Debugf("trying to validate recovery code again")
			continue

		case <-c.c:
			log.Debug("validating recovery code canceled")
			return nil, nil

		case <-overallTimeout:
			return nil, log.Error("validating recovery code timed out")
		}
	}

}

func getProClient() (*proclient.Client, error) {
	rt, ok := fronted.NewDirect(frontedAvailableTimeout)
	if !ok {
		return nil, log.Errorf("timed out waiting for fronting to finish configuring")
	}

	pc := proclient.NewClient(&http.Client{
		Transport: rt,
	}, func(req *http.Request, uc common.UserConfig) {
		common.AddCommonHeaders(uc, req)
	})

	return pc, nil
}
