package pro

import (
	"sync"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/pro/client"
)

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

// GetUserDataFast gets the user data for the given userID if found.
func GetUserDataFast(userID int64) (*client.User, bool) {
	return userData.get(userID)
}

func WaitForUserData(userID int64) *client.User {
	return userData.wait(userID)
}

// IsProUser indicates whether or not the user is pro, calling the Pro API if
// necessary to determine the status.
func IsProUser(userID int64, proToken string, deviceID string) (isPro bool, statusKnown bool) {
	user, err := GetUserData(userID, proToken, deviceID)
	if err != nil {
		log.Errorf("Error getting user status: %v", err)
		return false, false
	}
	return IsActive(user.UserStatus), true
}

// IsProUserFast indicates whether or not the user is pro and whether or not the
// user's status is know, never calling the Pro API to determine the status.
func IsProUserFast(userID int64) (isPro bool, statusKnown bool) {
	user, found := GetUserDataFast(userID)
	return IsActive(user.UserStatus), found
}

// IsActive determines whether the given status is an active status
func IsActive(status string) bool {
	return status == "active"
}

//NewUser creates a new user via Pro API, and updates local cache.
func NewUser(deviceID string) (*client.User, error) {
	log.Debugf("Creating new user with device ID '%v'", deviceID)
	user := client.User{Auth: client.Auth{
		DeviceID: deviceID,
	}}
	resp, err := client.NewClient(GetHTTPClient()).UserCreate(user)
	if err != nil {
		return nil, err
	}
	user = resp.User
	userID := user.Auth.ID
	setUserData(userID, &user)
	log.Debugf("created user %d", userID)
	return &user, nil
}

//GetUserData gets user data from Pro API, and updates local cache.
func GetUserData(userID int64, proToken string, deviceID string) (*client.User, error) {
	user, found := GetUserDataFast(userID)
	if found {
		return user, nil
	}
	log.Debugf("Fetching user status with device ID '%v', user ID '%v' and proToken %v", deviceID, userID, proToken)
	user = &client.User{Auth: client.Auth{
		DeviceID: deviceID,
		ID:       userID,
		Token:    proToken,
	}}
	resp, err := client.NewClient(GetHTTPClient()).UserStatus(*user)
	if err != nil {
		return nil, err
	}
	setUserData(userID, &resp.User)
	log.Debugf("User %d is '%v'", userID, resp.User.UserStatus)
	return &resp.User, nil
}

func setUserData(userID int64, user *client.User) {
	userData.save(userID, user)
}
