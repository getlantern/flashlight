package client

import (
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
)

func generateDeviceId() string {
	return uuid.New()
}

func generateUser() *common.UserConfigData {
	return common.NewUserConfigData(generateDeviceId(), 0, "", nil, "en-US")
}

var (
	userA *common.UserConfigData
	userB *common.UserConfigData
)

var tc *Client

func init() {
	common.ForceStaging()
}

func TestCreateClient(t *testing.T) {
	tc = NewClient(nil, func(r *http.Request, uc common.UserConfig) {
		common.AddHeadersForInternalServices(r, uc, true)
	})
}

func TestCreateUserA(t *testing.T) {
	userA = generateUser()

	res, err := tc.UserCreate(userA)
	assert.NoError(t, err)

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Expiration == 0)
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Code != "")
	assert.True(t, res.User.Referral == res.User.Code)

	userA.UserID = res.User.ID
	userA.Token = res.User.Token
}

func TestUserAData(t *testing.T) {
	res, err := tc.UserData(userA)
	assert.NoError(t, err)
	assert.Equal(t, "ok", res.Status)
}

func TestCreateUserB(t *testing.T) {
	userB = generateUser()

	res, err := tc.UserCreate(userB)
	assert.NoError(t, err)

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Code != "")
	assert.True(t, res.User.Referral == res.User.Code)

	userB.UserID = res.User.ID
	userB.Token = res.User.Token
}
