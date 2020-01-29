package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("pro-server-client")

	defaultTimeout = time.Second * 30
	maxRetries     = 4
	retryBaseTime  = time.Millisecond * 100
)

var (
	ErrAPIUnavailable = errors.New("API unavailable.")
)

type baseResponse interface {
	status() string
	error() string
}

type BaseResponse struct {
	Status  string `json:"status"`
	Error   string `json:"error"`
	ErrorId string `json:"errorId"`
}

func (resp BaseResponse) status() string {
	return resp.Status
}

func (resp BaseResponse) error() string {
	return resp.Error
}

type UserDataResponse struct {
	BaseResponse
	User `json:",inline"`
}

type LinkResponse struct {
	BaseResponse
	UserID   int    `json:"userID"`
	ProToken string `json:"token"`
}

type LinkCodeResponse struct {
	BaseResponse
	Code     string
	ExpireAt int64
}

type Client struct {
	httpClient *http.Client
	preparePro func(*http.Request, common.UserConfig)
}

// NewClient creates a new pro client.
func NewClient(httpClient *http.Client, preparePro func(r *http.Request, uc common.UserConfig)) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultTimeout,
		}
	}
	return &Client{httpClient: httpClient, preparePro: preparePro}
}

// UserCreate creates an user without asking for any payment.
func (c *Client) UserCreate(user common.UserConfig) (res *UserDataResponse, err error) {
	body := strings.NewReader(url.Values{"locale": {user.GetLanguage()}}.Encode())
	req, err := http.NewRequest(http.MethodPost, "https://"+common.ProAPIHost+"/user-create", body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	payload, err := c.do(user, req)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// UserCreateWithID creates a new user with a randomly generated UUID on the
// client without asking for any payment.
func (c *Client) UserCreateWithID(user common.UserConfig, userID string) (res *UserDataResponse, err error) {
	vals := url.Values{
		"locale": {user.GetLanguage()},
		"userID": {userID},
	}
	body := strings.NewReader(vals.Encode())
	req, err := http.NewRequest(http.MethodPost,
		"https://"+common.ProAPIHost+"/user-create-with-id", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	payload, err := c.do(user, req)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// UserData Returns all user data, including payments, referrals and all
// available fields.
func (c *Client) UserData(user common.UserConfig) (*UserDataResponse, error) {
	query := url.Values{
		"timeout": {"10"},
		"locale":  {user.GetLanguage()},
	}

	resp := &UserDataResponse{}
	if err := c.execute(user, http.MethodGet, "user-data", query, resp); err != nil {
		log.Errorf("Failed to get user data: %v", err)
		return nil, err
	}

	return resp, nil
}

// RecoverProAccount attempts to recover an existing Pro account linked to this email address and device ID
func (c *Client) RecoverProAccount(user common.UserConfig, emailAddress string) (*LinkResponse, error) {
	query := url.Values{
		"email":  {emailAddress},
		"locale": {user.GetLanguage()},
	}

	resp := &LinkResponse{}
	if err := c.execute(user, http.MethodPost, "user-recover", query, resp); err != nil {
		log.Errorf("Failed to recover pro user: %v", err)
		return nil, err
	}

	return resp, nil
}

// RequestRecoveryEmail requests an account recovery email for linking to an existing pro account
func (c *Client) RequestRecoveryEmail(user common.UserConfig, deviceName, emailAddress string) (err error) {
	query := url.Values{
		"email":      {emailAddress},
		"deviceName": {deviceName},
		"locale":     {user.GetLanguage()},
	}

	resp := &BaseResponse{}
	if err := c.execute(user, http.MethodPost, "user-link-request", query, resp); err != nil {
		log.Errorf("Failed to request a recovery code: %v", err)
		return err
	}

	return nil
}

// ValidateRecoveryCode validates the given recovery code and finishes linking the device, returning the user_id and pro_token for the account.
func (c *Client) ValidateRecoveryCode(user common.UserConfig, code string) (*LinkResponse, error) {
	query := url.Values{
		"code":   {code},
		"locale": {user.GetLanguage()},
	}

	resp := &LinkResponse{}
	if err := c.execute(user, http.MethodPost, "user-link-validate", query, resp); err != nil {
		log.Errorf("Failed to validate recovery code: %v", err)
		return nil, err
	}

	return resp, nil
}

// RequestDeviceLinkingCode requests a new device linking code to allow linking the current device to a pro account via an existing pro device.
func (c *Client) RequestDeviceLinkingCode(user common.UserConfig, deviceName string) (*LinkCodeResponse, error) {
	query := url.Values{
		"deviceName": {deviceName},
		"locale":     {user.GetLanguage()},
	}

	resp := &LinkCodeResponse{}
	if err := c.execute(user, http.MethodPost, "link-code-request", query, resp); err != nil {
		log.Errorf("Failed to get link code: %v", err)
		return nil, err
	}

	return resp, nil
}

// ValidateDeviceLinkingCode validates a device linking code to allow linking the current device to a pro account via an existing pro device.
func (c *Client) ValidateDeviceLinkingCode(user common.UserConfig, deviceName, code string) (*LinkResponse, error) {
	query := url.Values{
		"code":       {code},
		"deviceName": {deviceName},
		"locale":     {user.GetLanguage()},
	}

	resp := &LinkResponse{}
	if err := c.execute(user, http.MethodPost, "link-code-redeem", query, resp); err != nil {
		log.Errorf("Failed to validate link code: %v", err)
		return nil, err
	}

	return resp, nil
}

func (c *Client) do(user common.UserConfig, req *http.Request) ([]byte, error) {
	var buf []byte
	if req.Body != nil {
		var err error
		buf, err = ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
	}

	c.preparePro(req, user)

	for i := 0; i < maxRetries; i++ {
		client := c.httpClient
		log.Debugf("%s %s", req.Method, req.URL)
		if len(buf) > 0 {
			req.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
		}

		res, err := client.Do(req)
		if err == nil {
			defer res.Body.Close()
			switch res.StatusCode {
			case 200:
				body, err := ioutil.ReadAll(res.Body)
				return body, err
			case 202:
				log.Debugf("Received 202, retrying idempotent operation immediately.")
				continue
			default:
				body, err := ioutil.ReadAll(res.Body)
				if err == nil {
					log.Debugf("Expecting 200, got: %d, body: %v", res.StatusCode, string(body))
				} else {
					log.Debugf("Expecting 200, got: %d, could not get body: %v", res.StatusCode, err)
				}
			}
		} else {
			log.Debugf("Do: %v, res: %v", err, res)
		}

		retryTime := time.Duration(math.Pow(2, float64(i))) * retryBaseTime
		log.Debugf("failed, waiting %v to retry.", retryTime)
		time.Sleep(retryTime)
	}
	return nil, ErrAPIUnavailable
}

func (c *Client) execute(user common.UserConfig, method, path string, query url.Values, resp baseResponse) error {
	req, err := http.NewRequest(method, "https://"+common.ProAPIHost+"/"+path, nil)
	if err != nil {
		return err
	}

	query["locale"] = []string{user.GetLanguage()}
	req.URL.RawQuery = query.Encode()

	payload, err := c.do(user, req)
	if err != nil {
		return err
	}

	err = json.Unmarshal(payload, &resp)
	if err != nil {
		return err
	}

	if resp.status() == "error" {
		return errors.New(resp.error())
	}

	return nil
}
