package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/flashlight/v7/common"
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

type plansResponse struct {
	BaseResponse
	*PlansResponse `json:",inline"`
}

// Plans returns a list of Pro plans
func (c *Client) Plans(user common.UserConfig) (*plansResponse, error) {
	query := url.Values{
		"locale": {user.GetLanguage()},
	}

	resp := &plansResponse{PlansResponse: &PlansResponse{}}
	if err := c.execute(user, http.MethodGet, "plans", query, resp); err != nil {
		log.Errorf("Failed to fetch plans: %v", err)
		return nil, err
	}

	return resp, nil
}

type paymentRedirectResponse struct {
	BaseResponse
	*PaymentRedirectResponse `json:",inline"`
}

// PaymentRedirect is called when the continue to payment button is clicked and returns a redirect URL
func (c *Client) PaymentRedirect(user common.UserConfig, req *PaymentRedirectRequest) (*paymentRedirectResponse, error) {
	query := url.Values{
		"countryCode": {req.CountryCode},
		"deviceName":  {req.DeviceName},
		"email":       {req.Email},
		"plan":        {req.Plan},
		"provider":    {req.Provider},
	}

	b, _ := json.Marshal(user)
	log.Debugf("User config is %v", string(b))

	resp := &paymentRedirectResponse{PaymentRedirectResponse: &PaymentRedirectResponse{}}
	if err := c.execute(user, http.MethodGet, "payment-redirect", query, resp); err != nil {
		log.Errorf("Failed to fetch payment redirect: %v", err)
		return nil, err
	}
	log.Debugf("Redirect is %s", resp.Redirect)
	return resp, nil
}

type paymentMethodsResponse struct {
	*PaymentMethodsResponse `json:",inline"`
	BaseResponse
}

// PaymentMethodsV3 returns a list of payment options available to the given user
func (c *Client) PaymentMethodsV3(user common.UserConfig) (*paymentMethodsResponse, error) {
	query := url.Values{
		"locale": {user.GetLanguage()},
	}

	resp := &paymentMethodsResponse{PaymentMethodsResponse: &PaymentMethodsResponse{}}
	if err := c.execute(user, http.MethodGet, "plans-v3", query, resp); err != nil {
		log.Errorf("Failed to fetch payment methods: %v", err)
		return nil, err
	}
	return resp, nil
}

// PaymentMethodsV3 returns a list of payment, plans and icons options available to the given user
func (c *Client) PaymentMethodsV4(user common.UserConfig) (*paymentMethodsResponse, error) {
	query := url.Values{
		"locale": {user.GetLanguage()},
	}

	resp := &paymentMethodsResponse{PaymentMethodsResponse: &PaymentMethodsResponse{}}
	if err := c.execute(user, http.MethodGet, "plans-v4", query, resp); err != nil {
		log.Errorf("Failed to fetch payment methods-v4: %v", err)
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

// EmailExists checks whether a Pro account exists with the given email address
func (c *Client) EmailExists(user common.UserConfig, emailAddress string) error {
	query := url.Values{
		"email": {emailAddress},
	}

	resp := &BaseResponse{}
	if err := c.execute(user, http.MethodGet, "email-exists", query, resp); err != nil {
		log.Errorf("Failed to check if email exists: %v", err)
		return err
	}

	return nil
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

// LinkCodeApprove approves a device linking code when requesting to use a device with a Pro account
func (c *Client) LinkCodeApprove(user common.UserConfig, code string) (*BaseResponse, error) {
	query := url.Values{
		"code":   {code},
		"locale": {user.GetLanguage()},
	}

	var resp BaseResponse
	if err := c.execute(user, http.MethodPost, "link-code-approve", query, &resp); err != nil {
		log.Errorf("Failed to approve link code: %v", err)
		return nil, err
	}

	return &resp, nil
}

// DeviceRemove removes the device with the given ID from a user's Pro account
func (c *Client) DeviceRemove(user common.UserConfig, deviceID string) (*LinkResponse, error) {
	query := url.Values{
		"deviceID": {deviceID},
		"locale":   {user.GetLanguage()},
	}

	var resp LinkResponse
	if err := c.execute(user, http.MethodPost, "user-link-remove", query, &resp); err != nil {
		log.Errorf("Failed to remove link code: %v", err)
		return nil, err
	}

	return &resp, nil
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

// MigrateDeviceID migrates from the old device ID scheme to the new
func (c *Client) MigrateDeviceID(user common.UserConfig, oldDeviceID string) error {
	query := url.Values{
		"oldDeviceID": {oldDeviceID},
	}

	resp := &BaseResponse{}
	return c.execute(user, http.MethodPost, "migrate-device-id", query, resp)
}

// RedeemResellerCode redeems a reseller code for the given user
//
// Note: In reality, the response for this route from pro-server is not
// BaseResponse but of this type
// https://github.com/getlantern/pro-server-neu/blob/34bcdc042e983bf9504014aa066bba6bdedcebdb/handlers/purchase.go#L201.
// That being said, we don't really care about the response from pro-server
// here. We just wanna know if it succeeded or failed, which is encapsulated in the fields of BaseResponse.
func (c *Client) RedeemResellerCode(user common.UserConfig, emailAddress, resellerCode, deviceName, currency string) (*BaseResponse, error) {
	query := url.Values{
		"email":          {emailAddress},
		"resellerCode":   {resellerCode},
		"idempotencyKey": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		"currency":       {currency},
		"deviceName":     {deviceName},
		"provider":       {"reseller-code"},
	}

	resp := &BaseResponse{}
	if err := c.execute(user, http.MethodPost, "purchase", query, resp); err != nil {
		log.Errorf("Failed to redeem reseller code: %v", err)
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

	req.Header.Set("Referer", "http://localhost:37457/")

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
