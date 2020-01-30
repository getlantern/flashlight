package pro

import (
	"net/http"
	"sync"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-server/uuid"
)

var logger = golog.LoggerFor("flashlight.app.pro")

type userMap struct {
	sync.RWMutex
	data       map[string]eventual.Value
	onUserData []func(current *client.User, new *client.User)
}

var userData = userMap{
	data:       make(map[string]eventual.Value),
	onUserData: make([]func(current *client.User, new *client.User), 0),
}

// OnUserData allows registering an event handler to learn when the
// user data has been fetched.
func OnUserData(cb func(current *client.User, new *client.User)) {
	userData.Lock()
	userData.onUserData = append(userData.onUserData, cb)
	userData.Unlock()
}

// OnProStatusChange allows registering an event handler to learn when the
// user's pro status or "yinbi enabled" status has changed.
func OnProStatusChange(cb func(isPro bool, yinbiEnabled bool)) {
	OnUserData(func(current *client.User, new *client.User) {
		if current == nil ||
			isActive(current.UserStatus) != isActive(new.UserStatus) ||
			current.YinbiEnabled != new.YinbiEnabled {
			cb(isActive(new.UserStatus), new.YinbiEnabled)
		}
	})
}

func (m *userMap) save(userID string, u *client.User) {
	m.Lock()
	v := m.data[userID]
	var current *client.User
	if v == nil {
		v = eventual.NewValue()
	} else {
		cur, _ := v.Get(0)
		current, _ = cur.(*client.User)
	}
	v.Set(u)
	m.data[userID] = v
	onUserData := m.onUserData
	m.Unlock()
	for _, cb := range onUserData {
		cb(current, u)
	}
}

func (m *userMap) get(userID string) (*client.User, bool) {
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

// IsProUser indicates whether or not the user is pro, calling the Pro API if
// necessary to determine the status.
func IsProUser(uc common.UserConfig) (isPro bool, statusKnown bool) {
	user, found := GetUserDataFast(uc.GetUserID())
	if !found {
		var err error
		user, err = fetchUserDataWithClient(uc, httpClient)
		if err != nil {
			logger.Debugf("Got error fetching pro user: %v", err)
			return false, false
		}
	}
	return isActive(user.UserStatus), true
}

// IsProUserFast indicates whether or not the user is pro and whether or not the
// user's status is know, never calling the Pro API to determine the status.
func IsProUserFast(uc common.UserConfig) (isPro bool, statusKnown bool) {
	user, found := GetUserDataFast(uc.GetUserID())
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
func GetUserDataFast(userID string) (*client.User, bool) {
	return userData.get(userID)
}

// NewUser creates a new user via Pro API, and updates local cache.
func NewUser(uc common.UserConfig) (*client.User, error) {
	return newUserWithClient(uc, httpClient)
}

// newUserWithClient creates a new user via Pro API, and updates local cache
// using the specified http client.
func newUserWithClient(uc common.UserConfig, hc *http.Client) (*client.User, error) {
	deviceID := uc.GetDeviceID()
	// use deviceID, generate a random user ID, and token
	userID := uuid.Random()
	user := common.NewUserConfigData(deviceID, userID, "", uc.GetInternalHeaders(), uc.GetLanguage())
	logger.Debugf("Creating new user with device ID %v and userID %v",
		deviceID, userID)
	resp, err := client.NewClient(hc, PrepareProRequestWithOptions).UserCreateWithID(user, userID)
	if err != nil {
		return nil, err
	}
	setUserData(resp.User.Auth.ID, &resp.User)
	logger.Debugf("created user %+v", resp.User)
	return &resp.User, nil
}

// FetchUserData fetches user data from Pro API, and updates local cache.
func FetchUserData(uc common.UserConfig) (*client.User, error) {
	return fetchUserDataWithClient(uc, httpClient)
}

func fetchUserDataWithClient(uc common.UserConfig, hc *http.Client) (*client.User, error) {
	userID := uc.GetUserID()
	logger.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", uc.GetDeviceID(), userID, uc.GetToken())

	resp, err := client.NewClient(hc, PrepareProRequestWithOptions).UserData(uc)
	if err != nil {
		return nil, err
	}
	setUserData(userID, &resp.User)
	logger.Debugf("User %d is '%v'", userID, resp.User.UserStatus)
	return &resp.User, nil
}

func setUserData(userID string, user *client.User) {
	logger.Debugf("Storing user data for user %v", userID)
	userData.save(userID, user)
}
