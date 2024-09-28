package client

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/getlantern/flashlight/v7/common"
)

type mockTransport struct {
	Resp *http.Response
}

func (t *mockTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return t.Resp, nil
}

func createMockClient(resp *http.Response) *http.Client {
	return &http.Client{
		Transport: &mockTransport{
			Resp: resp,
		},
	}
}

func newErrorResponse(message string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":"%s","status":"error"}`, message))),
	}
}
func newResponse(data any) *http.Response {
	jsonResponse, _ := json.Marshal(data)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(jsonResponse))),
	}
}
func newUserDataResponse(data *common.UserConfigData) *http.Response {
	return newResponse(UserDataResponse{
		User: User{
			Auth: Auth{
				Token: data.Token,
				ID:    data.UserID,
			},
			Code:     "A",
			Referral: "A",
		},
	})
}

func newPlansResponse() *http.Response {
	return newResponse(plansResponse{
		PlansResponse: &PlansResponse{
			Plans: []*ProPlan{
				{
					Id: "test",
				},
			},
		},
	})
}

func newPaymentMethodsResponse() *http.Response {
	icons := map[string]*structpb.ListValue{}
	icons["a"] = nil
	providers := map[string]*structpb.ListValue{}
	providers["a"] = nil
	return newResponse(paymentMethodsResponse{
		PaymentMethodsResponse: &PaymentMethodsResponse{
			Icons: icons,
			Plans: []*ProPlan{
				{
					Id: "test",
				},
			},
			Providers: providers,
		},
	})
}

func newLinkResponse() *http.Response {
	return newResponse(LinkCodeResponse{
		Code:     "123456",
		ExpireAt: time.Now().Add(5 * time.Minute).Unix(),
	})
}

func generateDeviceId() string {
	return uuid.New()
}

func generateUser() *common.UserConfigData {
	return common.NewUserConfigData(common.DefaultAppName, generateDeviceId(), int64(rand.Uint64()), fmt.Sprintf("aasfge%d", rand.Uint64()), nil, "en-US")
}

func createClient(resp *http.Response) *Client {
	mockedHTTPClient := createMockClient(resp)
	return NewClient(mockedHTTPClient, func(req *http.Request, uc common.UserConfig) {
		common.AddCommonHeaders(uc, req)
	})
}

func TestCreateUser(t *testing.T) {
	user := generateUser()

	res, err := createClient(newUserDataResponse(user)).UserCreate(user)
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
	res, err := createClient(newUserDataResponse(user)).UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	// fetch this user's info with a new client
	res, err = createClient(newUserDataResponse(user)).UserData(user)
	if assert.NoError(t, err) {
		assert.True(t, res.User.ID != 0)
		assert.Equal(t, res.User.ID, user.UserID)
	}
}

func TestGetPlans(t *testing.T) {
	user := generateUser()
	res, err := createClient(newUserDataResponse(user)).UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	plansRes, err := createClient(newPlansResponse()).Plans(user)
	if !assert.NoError(t, err) {
		return
	}
	fmt.Println(plansRes.PlansResponse.Plans)
	assert.True(t, len(plansRes.PlansResponse.Plans) > 0)
}

func TestPaymentMethodsV4(t *testing.T) {
	user := generateUser()
	res, err := createClient(newUserDataResponse(user)).UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	paymentResp, err := createClient(newPaymentMethodsResponse()).PaymentMethodsV4(user)
	if !assert.NoError(t, err) {
		return
	}
	fmt.Println(paymentResp.PaymentMethodsResponse)
	assert.True(t, len(paymentResp.PaymentMethodsResponse.Icons) > 0)
	assert.True(t, len(paymentResp.PaymentMethodsResponse.Plans) > 0)
	assert.True(t, len(paymentResp.PaymentMethodsResponse.Providers) > 0)
}

func TestUserDataMissing(t *testing.T) {
	user := generateUser()

	_, err := createClient(newErrorResponse("invalid user")).UserData(user)
	assert.Error(t, err)
}

func TestUserDataWrong(t *testing.T) {
	user := generateUser()
	user.UserID = -1
	user.Token = "nonsense"

	_, err := createClient(newErrorResponse("Not authorized")).UserData(user)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "Not authorized")
	}
}

func TestRequestDeviceLinkingCode(t *testing.T) {
	user := generateUser()
	res, err := createClient(newUserDataResponse(user)).UserCreate(user)
	if !assert.NoError(t, err) {
		return
	}
	user.UserID = res.User.ID
	user.Token = res.User.Token

	lcr, err := createClient(newLinkResponse()).RequestDeviceLinkingCode(user, "Test Device")
	if assert.NoError(t, err) {
		assert.NotEmpty(t, lcr.Code)
		assert.True(t, time.Unix(lcr.ExpireAt, 0).After(time.Now()))
	}
}

func TestCreateUniqueUsers(t *testing.T) {
	userA := generateUser()
	res, err := createClient(newUserDataResponse(userA)).UserCreate(userA)
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Token != "")
	userA.UserID = res.User.ID
	userA.Token = res.User.Token

	userB := generateUser()
	res, err = createClient(newUserDataResponse(userB)).UserCreate(userB)
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
