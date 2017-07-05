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

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("pro-server-client")

	defaultTimeout = time.Second * 30
	maxRetries     = 12
	retryBaseTime  = time.Millisecond * 100
	endpointPrefix = "https://" + common.ProAPIHost
)

const defaultLocale = "en_US"

var (
	ErrAPIUnavailable = errors.New("API unavailable.")
)

type Client struct {
	httpClient *http.Client
	locale     string
}

func (c *Client) get(endpoint string, header http.Header, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}

	params.Set("timeout", "10")
	params.Set("locale", c.locale)

	encodedParams := params.Encode()

	if encodedParams != "" {
		encodedParams = "?" + encodedParams
	}

	req, err := http.NewRequest("GET", endpointPrefix+endpoint+encodedParams, nil)
	if err != nil {
		return nil, err
	}
	if req.Header == nil {
		req.Header = http.Header{}
	}
	for k := range header {
		req.Header[k] = header[k]
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	var buf []byte
	if req.Body != nil {
		var err error
		buf, err = ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
	}
	req.Header.Set("User-Agent", "Lantern-Android-"+common.Version)

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
				// Accepted: Immediately retry idempotent operation
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
		log.Debugf("timed out, waiting %v to retry.", retryTime)
		time.Sleep(retryTime)
	}
	return nil, ErrAPIUnavailable
}

func (c *Client) post(endpoint string, header http.Header, post url.Values) ([]byte, error) {
	if post == nil {
		post = url.Values{}
	}
	post.Set("locale", c.locale)

	req, err := http.NewRequest("POST", endpointPrefix+endpoint, strings.NewReader(post.Encode()))
	if err != nil {
		return nil, err
	}
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k := range header {
		req.Header[k] = header[k]
	}
	return c.do(req)
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultTimeout,
		}
	}
	return &Client{locale: defaultLocale, httpClient: httpClient}
}

func (c *Client) SetLocale(locale string) {
	c.locale = locale
}

// UserCreate creates an user without asking for any payment.
func (c *Client) UserCreate(user User) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-create`, http.Header{
		common.DeviceIdHeader: {user.Auth.DeviceID},
	}, nil)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

func (c *Client) UserUpdate(user User, email string) (res *Response, userId int64, err error) {
	var payload []byte
	var updatedId UserId
	payload, err = c.post(`/user-update`, user.headers(),
		url.Values{
			"email": {email},
		},
	)
	if err != nil {
		return nil, 0, err
	}
	err = json.Unmarshal(payload, &res)
	if err == nil {
		// user-update sometimes returns a string userId unlike other calls
		// convert it here (if possible)
		err = json.Unmarshal(payload, &updatedId)
		if err == nil {
			userId, _ = strconv.ParseInt(updatedId.UserId, 10, 64)
		} else {
			return res, 0, nil
		}
	}
	return
}

func (c *Client) EmailExists(user User, email string) (res *Response, err error) {
	var payload []byte
	payload, err = c.get(`/email-exists`, user.headers(),
		url.Values{
			"email": {email},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// UserLinkConfigure allows the client to initiate the configuration of a
// verified method of authenticating a user.
func (c *Client) UserLinkConfigure(user User) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-link-configure`, user.headers(),
		url.Values{
			"telephone": {user.PhoneNumber},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// CancelSubscription cancels the subscription.
func (c *Client) CancelSubscription(user User) (*Response, error) {
	return c.SubscriptionUpdate(user, "cancel")
}

// SubscriptionUpdate changes the next billable term to the requested
// subscription Id. It is used also to cancel a subscription, by providing the
// subscription Id cancel.
func (c *Client) SubscriptionUpdate(user User, subscriptionId string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/subscription-update`, user.headers(),
		url.Values{
			"plan": {subscriptionId},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// UserEmailRequest is used to verify a new device - account recovery
func (c *Client) UserEmailRequest(user User, email string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-email-request`, user.headers(),
		url.Values{
			"email": {email},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// UserLinkRequest Perform device linking or user recovery.
func (c *Client) UserLinkRequest(user User, email, deviceName string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-link-request`, user.headers(),
		url.Values{
			"email":      {email},
			"deviceName": {deviceName},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// allows the client to initiate the configuration of a
// verified method of authenticating a user.
func (c *Client) UserLinkValidate(user User, code string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-link-validate`, user.headers(),
		url.Values{
			"code": {code},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// RedeemReferralCode redeems a referral code.
func (c *Client) UserRecover(user User, key string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-recover`, user.headers(),
		url.Values{
			"email": {key},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// RedeemReferralCode redeems a referral code.
func (c *Client) RedeemReferralCode(user User, referralCode string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/referral-attach`, user.headers(),
		url.Values{
			"code": {referralCode},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// RequestLinkCode Perform device linking or user recovery.
func (c *Client) RequestLinkCode(user User, deviceName string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/link-code-request`, user.headers(),
		url.Values{
			"deviceName": {deviceName},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

func (c *Client) ApplyLinkCode(user User) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/link-code-approve`, user.headers(),
		url.Values{
			"code": {user.Code},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

func (c *Client) RedeemLinkCode(user User, deviceName string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/link-code-redeem`, user.headers(),
		url.Values{
			"code":       {user.Code},
			"deviceName": {deviceName},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

func (c *Client) UserLinkRemove(user User, deviceId string) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/user-link-remove`, user.headers(),
		url.Values{
			"deviceID": {deviceId},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

func (c *Client) UserStatus(user User) (*Response, error) {
	res, err := c.UserData(user)
	if err != nil {
		log.Errorf("Failed to get user data: %v", err)
		return nil, err
	}

	if res.Status == "error" {
		return nil, errors.New(res.Error)
	}
	return res, nil
}

// UserData Returns all user data, including payments, referrals and all
// available fields.
func (c *Client) UserData(user User) (res *Response, err error) {
	var payload []byte
	payload, err = c.get(`/user-data`, user.headers(), nil)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

func (c *Client) PWSignature(user User, planId string) (string, error) {

	payload, err := c.get(`/paymentwall-mobile-signature`, user.headers(), url.Values{
		"plan": {planId},
	})
	if err != nil {
		return "", err
	}
	sig := string(payload)
	return sig, nil
}

// Plans creates an user without asking for any payment.
func (c *Client) Plans(user User) (res *Response, err error) {
	var payload []byte
	payload, err = c.get(`/plans`, user.headers(), nil)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// Purchase single endpoint used for performing purchases.
func (c *Client) Purchase(user User, deviceName, pubKey string, purchase Purchase) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/purchase`, user.headers(),
		url.Values{
			"resellerCode":    {purchase.ResellerCode},
			"provider":        {purchase.Provider},
			"stripeToken":     {purchase.StripeToken},
			"stripeEmail":     {purchase.StripeEmail},
			"stripePublicKey": {pubKey},
			"email":           {purchase.Email},
			"idempotencyKey":  {purchase.IdempotencyKey},
			"plan":            {purchase.Plan},
			"currency":        {purchase.Currency},
			"deviceName":      {deviceName},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// TokenReset Request a token change. This will generate a new one and send it
// to the requesting device.
func (c *Client) TokenReset(user User) (res *Response, err error) {
	var payload []byte
	payload, err = c.post(`/token-reset`, user.headers(), nil)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}

// ChargeByID Request payment details by id.
func (c *Client) ChargeByID(user User, chargeID string) (res *Response, err error) {
	var payload []byte
	payload, err = c.get(`/charge-by-id`, user.headers(),
		url.Values{
			"changeId": {chargeID},
		},
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}
