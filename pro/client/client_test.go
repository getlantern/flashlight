package client

import (
	"net/http"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/v7/common"
)

func generateDeviceId() string {
	return uuid.New()
}

func generateUser() *common.UserConfigData {
	return common.NewUserConfigData(common.DefaultAppName, generateDeviceId(), 0, "", nil, "en-US")
}

func init() {
	common.ForceStaging()
}

func createClient() *Client {
	return NewClient(nil, func(req *http.Request, uc common.UserConfig) {
		common.AddCommonHeaders(uc, req)
	})
}

func TestCreateUser(t *testing.T) {
	user := generateUser()

	res, err := createClient().UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Expiration == 0)
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Code != "")
	assert.True(t, res.User.Referral == res.User.Code)
}

func TestGetUserData(t *testing.T) {
	user := generateUser()
	res, err := createClient().UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	// fetch this user's info with a new client
	res, err = createClient().UserData(user)
	if assert.NoError(t, err) {
		assert.True(t, res.User.ID != 0)
		assert.Equal(t, res.User.ID, user.UserID)
		// This is not set currently, this could be enabled if status is returned again ...
		// See https://github.com/getlantern/pro-server-neu/commit/cda2f9565bd1fde16334e57e00e2b5572423880c
		// assert.Equal(t, "ok", res.Status)
	}
}

func TestUserDataMissing(t *testing.T) {
	user := generateUser()

	_, err := createClient().UserData(user)
	assert.Error(t, err)
}

func TestUserDataWrong(t *testing.T) {
	user := generateUser()
	user.UserID = -1
	user.Token = "nonsense"

	_, err := createClient().UserData(user)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "Not authorized")
	}
}

func TestRequestDeviceLinkingCode(t *testing.T) {
	user := generateUser()
	res, err := createClient().UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	lcr, err := createClient().RequestDeviceLinkingCode(user, "Test Device")
	if assert.NoError(t, err) {
		assert.NotEmpty(t, lcr.Code)
		assert.True(t, time.Unix(lcr.ExpireAt, 0).After(time.Now()))
	}
}

func TestCreateUniqueUsers(t *testing.T) {
	userA := generateUser()
	res, err := createClient().UserCreate(userA)
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Token != "")
	userA.UserID = res.User.ID
	userA.Token = res.User.Token

	userB := generateUser()
	res, err = createClient().UserCreate(userB)
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Token != "")
	userB.UserID = res.User.ID
	userB.Token = res.User.Token

	assert.NotEqual(t, userA.UserID, userB.UserID)
	assert.NotEqual(t, userA.Token, userB.Token)
}
