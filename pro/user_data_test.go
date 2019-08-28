package pro

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/pro/client"
)

func TestUsers(t *testing.T) {
	common.ForceStaging()

	deviceID := "77777777"
	u, err := newUserWithClient(common.NewUserConfigData(deviceID, 0, "", nil, "en-US"), nil)

	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")
	assert.NotNil(t, u.Auth, "Should have gotten Auth")
	t.Logf("user: %+v", u)

	uc := common.NewUserConfigData(deviceID, u.Auth.ID, u.Auth.Token, nil, "en-US")
	u, err = fetchUserDataWithClient(uc, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	delete(userData.data, u.ID)

	u, err = fetchUserDataWithClient(uc, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	pro, _ := IsProUser(uc)
	assert.False(t, pro)
	pro, _ = IsProUserFast(uc)
	assert.False(t, pro)

	var waitUser int64 = 88888
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

	userData.save(waitUser, u)
	assert.Equal(t, 2, userDataSaved, "OnUserData should be called after each saving")
	assert.Equal(t, 1, changed, "OnProStatusChange should not be called again if nothing changes")

}
