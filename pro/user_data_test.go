package pro

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

func TestUsers(t *testing.T) {
	common.ForceStaging()

	deviceID := "77777777"
	u, err := newUserWithClient(deviceID, nil)

	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")
	t.Logf("user: %+v", u)

	auth := u.Auth
	auth.DeviceID = deviceID
	u, err = getUserDataWithClient(auth, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	delete(userData.data, u.ID)

	u, err = getUserDataWithClient(auth, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	pro, _ := IsProUser(auth)
	assert.False(t, pro)
	pro, _ = IsProUserFast(auth)
	assert.False(t, pro)

	user := userData.wait(u.ID)
	assert.NotNil(t, user)

	var userRef atomic.Value
	var waitUser int64 = 88888
	go func() {
		user8 := userData.wait(waitUser)
		userRef.Store(user8)
	}()

	userData.save(waitUser, u)
	time.Sleep(100 * time.Millisecond)

	assert.NotNil(t, userRef.Load())
}
