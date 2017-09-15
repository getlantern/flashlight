package pro

import (
	"net/http"
	"sync"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("flashlight.app.pro")

type userMap struct {
	sync.RWMutex
	data map[int64]eventual.Value
}

var userData = userMap{data: make(map[int64]eventual.Value)}

func (m *userMap) save(userID int64, u *client.User) {
	m.Lock()
	v := m.data[userID]
	if v == nil {
		v = eventual.NewValue()
	}
	v.Set(u)
	m.data[userID] = v
	m.Unlock()
}

func (m *userMap) get(userID int64) (*client.User, bool) {
	m.RLock()
	v := m.data[userID]
	m.RUnlock()
	if v == nil {
		return nil, false
	}
	u, valid := v.Get(0)
	if !valid {
		return nil, false
	}
	return u.(*client.User), true
}

func (m *userMap) wait(userID int64) *client.User {
	m.Lock()
	v := m.data[userID]
	if v == nil {
		v = eventual.NewValue()
		m.data[userID] = v
	}
	m.Unlock()
	u, _ := v.Get(-1)
	return u.(*client.User)
}

// IsProUser indicates whether or not the user is pro, calling the Pro API if
// necessary to determine the status.
func IsProUser() (isPro bool, statusKnown bool) {
	user, err := GetUserData(authConfig)
	if err != nil {
		return false, false
	}
	return isActive(user.UserStatus), true
}

// IsProUserFast indicates whether or not the user is pro and whether or not the
// user's status is know, never calling the Pro API to determine the status.
func IsProUserFast() (isPro bool, statusKnown bool) {
	user, found := GetUserDataFast(authConfig.GetUserID())
	if !found {
		return false, false
	}
	return isActive(user.UserStatus), found
}

// isActive determines whether the given status is an active status
func isActive(status string) bool {
	return status == "active"
}

// GetUserDataFast gets the user data for the given userID if found.
func GetUserDataFast(userID int64) (*client.User, bool) {
	return userData.get(userID)
}

// WaitForUserData blocks itself to get the user data for the given userID
// until it's available.
func WaitForUserData(userID int64) *client.User {
	return userData.wait(userID)
}

//NewUser creates a new user via Pro API, and updates local cache.
func NewUser(deviceID string) (*client.User, error) {
	return newUserWithClient(deviceID, httpClient)
}

// newUserWithClient creates a new user via Pro API, and updates local cache
// using the specified http client.
func newUserWithClient(deviceID string, hc *http.Client) (*client.User, error) {
	logger.Debugf("Creating new user with device ID '%v'", deviceID)
	user := client.User{Auth: client.Auth{
		DeviceID: deviceID,
	}}
	resp, err := client.NewClient(hc).UserCreate(user)
	if err != nil {
		return nil, err
	}
	setUserData(resp.User.Auth.ID, &resp.User)
	log.Debugf("created user %+v", resp.User)
	return &resp.User, nil
}

//GetUserData retrieves local cache first. If the data for the userID is not
//there, fetches from Pro API, and updates local cache.
func GetUserData(ac common.AuthConfig) (*client.User, error) {
	return getUserDataWithClient(ac, httpClient)
}

//getUserDataWithClient retrieves local cache first. If the data for the userID is not
//there, fetches from Pro API, and updates local cache.
func getUserDataWithClient(ac common.AuthConfig, hc *http.Client) (*client.User, error) {
	deviceID := ac.GetDeviceID()
	userID := ac.GetUserID()
	proToken := ac.GetToken()

	user, found := GetUserDataFast(userID)
	if found {
		return user, nil
	}
	logger.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user = &client.User{Auth: client.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	resp, err := client.NewClient(hc).UserStatus(*user)
	if err != nil {
		return nil, err
	}
	setUserData(userID, &resp.User)
	logger.Debugf("User %d is '%v'", userID, resp.User.UserStatus)
	return &resp.User, nil
}

func setUserData(userID int64, user *client.User) {
	logger.Debugf("Storing user data for user %v", userID)
	userData.save(userID, user)
}
