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
