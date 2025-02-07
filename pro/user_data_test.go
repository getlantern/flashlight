package pro

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/pro/client"
)

func TestUsers(t *testing.T) {
	deviceID := "77777777"
	userID := int64(1000)
	token := uuid.NewString()

	t.Run("newUserWithClient should create a user with success", func(t *testing.T) {
		mockedHTTPClient := createMockClient(newUserDataResponse(), nil)
		u, err := newUserWithClient(common.NewUserConfigData(common.DefaultAppName, deviceID, 0, "", nil, "en-US"), mockedHTTPClient)
		assert.NoError(t, err, "Unexpected error")
		assert.NotNil(t, u, "Should have gotten a user")
		t.Logf("user: %+v", u)
	})

	uc := common.NewUserConfigData(common.DefaultAppName, deviceID, userID, token, nil, "en-US")
	t.Run("fetchUserDataWithClient should fetch a user with success", func(t *testing.T) {
		mockedHTTPClient := createMockClient(newUserDataResponse(), nil)
		u, err := fetchUserDataWithClient(uc, mockedHTTPClient)
		assert.NoError(t, err, "Unexpected error")
		assert.NotNil(t, u, "Should have gotten a user")
		assert.NotNil(t, userData, "Should be user data")

		delete(userData.data, u.ID)
	})

	t.Run("status change should update when user data updated", func(t *testing.T) {
		mockedHTTPClient := createMockClient(newUserDataResponse(), nil)
		u, err := fetchUserDataWithClient(uc, mockedHTTPClient)
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
			userDataSaved++
		})

		OnProStatusChange(func(bool, bool) {
			changed++
		})

		userData.save(waitUser, u)
		assert.Equal(t, 1, userDataSaved, "OnUserData should be called")
		assert.Equal(t, 1, changed, "OnProStatusChange should be called")

		userData.save(waitUser, u)
		assert.Equal(t, 2, userDataSaved, "OnUserData should be called after each saving")
		assert.Equal(t, 1, changed, "OnProStatusChange should not be called again if nothing changes")
	})
}

type mockTransport struct {
	Resp *http.Response
	Err  error
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.Resp, t.Err
}

func createMockClient(resp *http.Response, err error) *http.Client {
	return &http.Client{
		Transport: &mockTransport{
			Resp: resp,
			Err:  err,
		},
	}
}

func newUserDataResponse() *http.Response {
	response := client.UserDataResponse{}
	jsonResponse, _ := json.Marshal(response)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(jsonResponse))),
	}
}
