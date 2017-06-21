package pro

import (
	"sync"
)

var (
	proStatusByUserID = make(map[int64]string)
	proStatusMx       sync.RWMutex
)

// SetProStatus updates the pro status for the given userID.
func SetProStatus(userID int64, status string) {
	proStatusMx.Lock()
	proStatusByUserID[userID] = status
	proStatusMx.Unlock()
}

// IsProUserFast indicates whether or not the user is pro and whether or not the
// user's status is know, never calling the Pro API to determine the status.
func IsProUserFast(userID int64) (isPro bool, statusKnown bool) {
	proStatusMx.RLock()
	status, found := proStatusByUserID[userID]
	proStatusMx.RUnlock()
	return IsActive(status), found
}

// IsActive determines whether the given status is an active status
func IsActive(status string) bool {
	return status == "active"
}
