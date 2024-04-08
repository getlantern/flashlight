package pro

import (
	"sync"

	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("flashlight.app.pro")

type userMap struct {
	sync.RWMutex
	data       map[int64]eventual.Value
	onUserData []func(current *User, new *User)
}

var userData = userMap{
	data:       make(map[int64]eventual.Value),
	onUserData: make([]func(current *User, new *User), 0),
}

// OnUserData allows registering an event handler to learn when the
// user data has been fetched.
func OnUserData(cb func(current *User, new *User)) {
	userData.Lock()
	userData.onUserData = append(userData.onUserData, cb)
	userData.Unlock()
}

// OnProStatusChange allows registering an event handler to learn when the
// user's pro status or "yinbi enabled" status has changed.
func OnProStatusChange(cb func(isPro bool, yinbiEnabled bool)) {
	OnUserData(func(current *User, new *User) {
		if current == nil ||
			isActive(current.UserStatus) != isActive(new.UserStatus) ||
			current.YinbiEnabled != new.YinbiEnabled {
			cb(isActive(new.UserStatus), new.YinbiEnabled)
		}
	})
}

func (m *userMap) save(userID int64, u *User) {
	m.Lock()
	v := m.data[userID]
	var current *User
	if v == nil {
		v = eventual.NewValue()
	} else {
		cur, _ := v.Get(0)
		current, _ = cur.(*User)
	}
	v.Set(u)
	m.data[userID] = v
	onUserData := m.onUserData
	m.Unlock()
	for _, cb := range onUserData {
		cb(current, u)
	}
}

func (m *userMap) get(userID int64) (*User, bool) {
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
	return u.(*User), true
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
func GetUserDataFast(userID int64) (*User, bool) {
	return userData.get(userID)
}
