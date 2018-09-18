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
	"github.com/getlantern/zaplog"
)

var (
	log = zaplog.LoggerFor("pro-server-client")

	defaultTimeout = time.Second * 30
	maxRetries     = 4
	retryBaseTime  = time.Millisecond * 100
)

const defaultLocale = "en_US"

var (
	ErrAPIUnavailable = errors.New("API unavailable.")
)

type Client struct {
	httpClient *http.Client
	locale     string
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
	common.AddCommonHeaders(user, req)

	for i := 0; i < maxRetries; i++ {
		client := c.httpClient
		log.Infof("%s %s", req.Method, req.URL)
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
				log.Infof("Received 202, retrying idempotent operation immediately.")
				continue
			default:
				body, err := ioutil.ReadAll(res.Body)
				if err == nil {
					log.Infof("Expecting 200, got: %d, body: %v", res.StatusCode, string(body))
				} else {
					log.Infof("Expecting 200, got: %d, could not get body: %v", res.StatusCode, err)
				}
			}
		} else {
			log.Infof("Do: %v, res: %v", err, res)
		}

		retryTime := time.Duration(math.Pow(2, float64(i))) * retryBaseTime
		log.Infof("timed out, waiting %v to retry.", retryTime)
		time.Sleep(retryTime)
	}
	return nil, ErrAPIUnavailable
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultTimeout,
		}
	}
	return &Client{locale: defaultLocale, httpClient: httpClient}
}

// UserCreate creates an user without asking for any payment.
func (c *Client) UserCreate(user common.UserConfig) (res *Response, err error) {
	body := strings.NewReader(url.Values{"locale": {c.locale}}.Encode())
	req, err := http.NewRequest("POST", "https://"+common.ProAPIHost+"/user-create", body)
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

func (c *Client) UserStatus(user common.UserConfig) (*Response, error) {
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
func (c *Client) UserData(user common.UserConfig) (res *Response, err error) {
	req, err := http.NewRequest("GET", "https://"+common.ProAPIHost+"/user-data", nil)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = url.Values{
		"timeout": {"10"},
		"locale":  {c.locale},
	}.Encode()

	payload, err := c.do(user, req)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(payload, &res)
	return
}
