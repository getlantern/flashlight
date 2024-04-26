package client

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/v7/common"
)

func generateDeviceId() string {
	return uuid.New()
}

func generateUser() *common.UserConfigData {
	return common.NewUserConfigData(common.DefaultAppName, generateDeviceId(), 35, "aasfge", nil, "en-US")
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
	}
}

func TestGetPlans(t *testing.T) {
	user := generateUser()
	res, err := createClient().UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	plansRes, err := createClient().Plans(user)
	if !assert.NoError(t, err) {
		return
	}
	fmt.Println(plansRes.PlansResponse.Plans)
	assert.True(t, len(plansRes.PlansResponse.Plans) > 0)
}

func TestPaymentMethodsV4(t *testing.T) {
	user := generateUser()
	res, err := createClient().UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	defer gock.Off()
	mockGetRequest("/plans-v4", expectedPlansV4Response)

	paymentResp, err := createClient().PaymentMethodsV4(user)
	if !assert.NoError(t, err) {
		return
	}
	fmt.Println(paymentResp.PaymentMethodsResponse)
	assert.True(t, len(paymentResp.PaymentMethodsResponse.Icons) > 0)
	assert.True(t, len(paymentResp.PaymentMethodsResponse.Plans) > 0)
	assert.True(t, len(paymentResp.PaymentMethodsResponse.Providers) > 0)
	assert.True(t, gock.IsDone())
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

// this method is used to mock the get request from the server
func mockGetRequest(path string, responseBody interface{}) {
	gock.New("https://" + common.ProAPIHost).Get(path).Reply(200).JSON(responseBody)
}

// this method is used to mock the post request from the server
func mockPostRequest(path string, responseBody interface{}) {
	gock.New("https://" + common.ProAPIHost).Post(path).Reply(200).JSON(responseBody)
}

// / Since now we stared using mock server, we need to update the expected response
const expectedPlansV4Response = `{"icons":{"paymentwall":["https://imagedelivery.net/sB6Q2DOuXQsFvrpwTks_kA/5fa28e16-5b78-44b1-f02b-2b80593b5c00/public"],"stripe":["https://imagedelivery.net/sB6Q2DOuXQsFvrpwTks_kA/e02a8c29-20a6-478f-ed01-e93d0dba0800/public","https://imagedelivery.net/sB6Q2DOuXQsFvrpwTks_kA/94870838-0129-417c-d65a-276422eac900/public","https://imagedelivery.net/sB6Q2DOuXQsFvrpwTks_kA/f56ce1a8-96d6-4528-fdc6-da4605f1c500/public"]},"plans":[{"id":"1y-usd-10","description":"One Year Plan","duration":{"days":0,"months":0,"years":1},"price":{"usd":4800},"expectedMonthlyPrice":{"usd":400},"usdPrice":4800,"usdPrice1Y":4800,"usdPrice2Y":8700,"redeemFor":{"days":0,"months":2},"renewalBonus":{"days":0,"months":1},"renewalBonusExpired":{"days":15,"months":0},"renewalBonusExpected":{"days":0,"months":0},"discount":0,"bestValue":false,"level":"pro"},{"id":"2y-usd-10","description":"Two Year Plan","duration":{"days":0,"months":0,"years":2},"price":{"usd":8700},"expectedMonthlyPrice":{"usd":363},"usdPrice":8700,"usdPrice1Y":4800,"usdPrice2Y":8700,"redeemFor":{"days":0,"months":2},"renewalBonus":{"days":0,"months":3},"renewalBonusExpired":{"days":15,"months":1},"renewalBonusExpected":{"days":0,"months":0},"discount":0.0925,"bestValue":true,"level":"pro"}],"providers":{"android":[{"method":"credit-card","providers":[{"name":"stripe","data":{"pubKey":"pk_live_4MSPfR6qNHMwjG86TZJv4NI0"}}]}],"desktop":[{"method":"credit-card","providers":[{"name":"stripe","data":{"pubKey":"pk_live_4MSPfR6qNHMwjG86TZJv4NI0"}}]},{"method":"paymentwall","providers":[{"name":"paymentwall"}]}]}}`
