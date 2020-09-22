package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getlantern/auth-server/models"
	"github.com/getlantern/auth-server/srp"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ui/api"
	"github.com/getlantern/flashlight/ui/testutils"
	scommon "github.com/getlantern/lantern-server/common"
	"github.com/getlantern/yinbi-server/utils"
	"github.com/stretchr/testify/assert"
)

type SRPTest struct {
	name         string
	user         models.UserParams
	endpoint     string
	hasError     bool
	expectedCode int
	expectedResp *api.ApiResponse
}

const TestPassword = "p@sswor1234!"

func getClient(t *testing.T, params *models.UserParams, h AuthHandler) (*models.UserParams, *srp.SRPClient) {
	req := createAuthRequest(params, loginEndpoint)
	params, client, err := h.GetSRPClient(req, true)
	assert.NoError(t, err, "Should be no error creating SRP client")
	assert.NotNil(t, client)
	assert.Equal(t, params.Password, "")
	assert.NotEmpty(t, params.Verifier)
	assert.NotEmpty(t, params.Credentials)
	return params, client
}

func createAuthRequest(params *models.UserParams, uri string) *http.Request {
	requestBody, _ := json.Marshal(params)
	req, _ := http.NewRequest(scommon.POST, uri, bytes.NewBuffer(requestBody))
	req.Header.Set(scommon.HeaderContentType, scommon.MIMEApplicationJSON)
	return req
}

func createUser() models.UserParams {
	username := utils.GenerateRandomString(12)
	email := fmt.Sprintf("%s@test.com", username)
	return models.UserParams{
		Email:    email,
		Username: username,
		Password: TestPassword,
	}
}

func TestSRP(t *testing.T) {
	h := New(api.Params{
		AuthServerAddr:  common.AuthServerAddr,
		YinbiServerAddr: common.YinbiServerAddr,
		HttpClient:      &http.Client{},
	})

	// Create new test user
	user := createUser()

	cases := []SRPTest{
		{
			"Register User",
			user,
			registrationEndpoint,
			false,
			http.StatusOK,
			nil,
		},
		{
			"Username Taken",
			models.UserParams{
				Username: user.Username,
				Email:    fmt.Sprintf("%s@test.com", utils.GenerateRandomString(12)),
				Password: user.Password,
			},
			registrationEndpoint,
			true,
			http.StatusBadRequest,
			&api.ApiResponse{
				Errors: map[string]string{
					"username": "username_taken",
				},
			},
		},
		{
			"Email Taken",
			models.UserParams{
				Username: utils.GenerateRandomString(12),
				Email:    user.Email,
				Password: user.Password,
			},
			registrationEndpoint,
			true,
			http.StatusBadRequest,
			&api.ApiResponse{
				Errors: map[string]string{
					"email": "email_taken",
				},
			},
		},
		{
			"Login User",
			models.UserParams{
				Username: user.Username,
				Password: TestPassword,
				Email:    user.Email,
			},
			loginEndpoint,
			false,
			http.StatusOK,
			nil,
		},
		{
			"Bad Login",
			models.UserParams{
				Username: user.Username,
				Password: "badpassword!234",
				Email:    user.Email,
			},
			loginEndpoint,
			true,
			http.StatusBadRequest,
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := createAuthRequest(&tc.user,
				tc.endpoint)
			rec := httptest.NewRecorder()
			h.authHandler(rec, req)
			testutils.DumpResponse(rec)
			if !tc.hasError {
				var resp models.AuthResponse
				testutils.DecodeResp(t, rec, &resp)
				assert.Equal(t, rec.Code, http.StatusOK)
				assert.NotEmpty(t, resp.User)
				assert.NotEmpty(t, resp.Credentials)
			} else {
				var resp api.ApiResponse
				testutils.DecodeResp(t, rec, &resp)
				assert.Equal(t, rec.Code, tc.expectedCode)
				if tc.expectedResp != nil {
					assert.Equal(t, resp, *tc.expectedResp)
				}
			}
		})
	}
}
