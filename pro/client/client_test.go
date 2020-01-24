package client

import (
	"net/http"
	"testing"
	"time"

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

func init() {
	common.ForceStaging()
}

func createClient() *Client {
	return NewClient(nil, func(req *http.Request, uc common.UserConfig) {
		common.AddCommonHeaders(uc, req)
	})
}

func TestCreateUserA(t *testing.T) {
	userA = generateUser()

	res, err := createClient().UserCreate(userA)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Expiration == 0)
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Code != "")
	assert.True(t, res.User.Referral == res.User.Code)

	userA.UserID = res.User.ID
	userA.Token = res.User.Token
}

func TestUserAData(t *testing.T) {
	res, err := createClient().UserData(userA)
	if assert.NoError(t, err) {
		assert.Equal(t, "ok", res.Status)
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
	res, err := createClient().RequestDeviceLinkingCode(userA, "Test Device")
	if assert.NoError(t, err) {
		assert.NotEmpty(t, res.Code)
		assert.True(t, time.Unix(res.ExpireAt, 0).After(time.Now()))
	}
}

func TestCreateUserB(t *testing.T) {
	userB = generateUser()

	res, err := createClient().UserCreate(userB)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Code != "")
	assert.True(t, res.User.Referral == res.User.Code)

	userB.UserID = res.User.ID
	userB.Token = res.User.Token
}
