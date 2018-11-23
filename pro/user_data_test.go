package pro

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
)

func TestUsers(t *testing.T) {
	common.ForceStaging()

	deviceID := "77777777"
	u, err := newUserWithClient(common.NewUserConfigData(deviceID, 0, "", nil), nil)

	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")
	t.Logf("user: %+v", u)

	uc := common.NewUserConfigData(deviceID, u.Auth.ID, u.Auth.Token, nil)
	u, err = getUserDataWithClient(uc, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	delete(userData.data, u.ID)

	u, err = getUserDataWithClient(uc, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	pro, _ := IsProUser(uc)
	assert.False(t, pro)
	pro, _ = IsProUserFast(uc)
	assert.False(t, pro)

	user := userData.wait(u.ID)
	assert.NotNil(t, user)

	var userRef atomic.Value
	var waitUser int64 = 88888
	go func() {
		user8 := userData.wait(waitUser)
		userRef.Store(user8)
	}()
	var changed int
	var userDataSaved int
	OnUserData(func(*client.User, *client.User) {
		userDataSaved += 1
	})

	OnProStatusChange(func(bool, bool) {
		changed += 1
	})

	userData.save(waitUser, u)
	assert.Equal(t, 1, userDataSaved, "OnUserData should be called")
	assert.Equal(t, 1, changed, "OnProStatusChange should be called")
	time.Sleep(100 * time.Millisecond)
	assert.NotNil(t, userRef.Load(), "wait should have succeeded")

	userData.save(waitUser, u)
	assert.Equal(t, 2, userDataSaved, "OnUserData should be called after each saving")
	assert.Equal(t, 1, changed, "OnProStatusChange should not be called again if nothing changes")

}
