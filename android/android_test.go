package android

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/integrationtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testProtector struct{}

type testSession struct {
	Session
	serializedInternalHeaders string
}

type testSettings struct {
	Settings
}

func (c testSettings) StickyConfig() bool       { return true }
func (c testSettings) DefaultDnsServer() string { return "8.8.8.8" }
func (c testSettings) TimeoutMillis() int       { return 15000 }
func (c testSettings) GetHttpProxyHost() string { return "127.0.0.1" }
func (c testSettings) GetHttpProxyPort() int    { return 49128 }

func (c testSession) AfterStart()                        {}
func (c testSession) BandwidthUpdate(int, int, int, int) {}
func (c testSession) ConfigUpdate(bool)                  {}
func (c testSession) ShowSurvey(survey string)           {}
func (c testSession) GetUserID() int64                   { return 0 }
func (c testSession) GetToken() string                   { return "" }
func (c testSession) GetForcedCountryCode() string       { return "" }
func (c testSession) GetDNSServer() string               { return "8.8.8.8" }
func (c testSession) SetStaging(bool)                    {}
func (c testSession) SetCountry(string)                  {}
func (c testSession) ProxyAll() bool                     { return true }
func (c testSession) GetDeviceID() string                { return "123456789" }
func (c testSession) AccountId() string                  { return "1234" }
func (c testSession) Locale() string                     { return "en-US" }
func (c testSession) GetTimeZone() string                { return "Americas/Chicago" }
func (c testSession) SetUserId(int64)                    {}
func (c testSession) SetToken(string)                    {}
func (c testSession) SetCode(string)                     {}
func (c testSession) IsProUser() bool                    { return true }

func (c testSession) UpdateStats(string, string, string, int, int) {}

func (c testSession) UpdateAdSettings(AdSettings) {}

func (c testSession) SerializedInternalHeaders() string {
	return c.serializedInternalHeaders
}

func TestProxying(t *testing.T) {

	listenPort := 24000
	nextListenAddr := func() string {
		listenPort++
		return fmt.Sprintf("localhost:%d", listenPort)
	}
	helper, err := integrationtest.NewHelper(t, nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr())
	if assert.NoError(t, err, "Unable to create temp configDir") {
		defer helper.Close()
		result, err := Start(helper.ConfigDir, "en_US", testSettings{}, testSession{})
		if assert.NoError(t, err, "Should have been able to start lantern") {
			newResult, err := Start("testapp", "en_US", testSettings{}, testSession{})
			if assert.NoError(t, err, "Should have been able to start lantern twice") {
				if assert.Equal(t, result.HTTPAddr, newResult.HTTPAddr, "2nd start should have resulted in the same address") {
					err := testProxiedRequest(helper, result.HTTPAddr, result.DNSGrabAddr, false)
					if assert.NoError(t, err, "Proxying request via HTTP should have worked") {
						err := testProxiedRequest(helper, result.SOCKS5Addr, result.DNSGrabAddr, true)
						assert.NoError(t, err, "Proxying request via SOCKS should have worked")
					}
				}
			}
		}
	}
}

func testProxiedRequest(helper *integrationtest.Helper, proxyAddr string, dnsGrabAddr string, socks bool) error {
	host := helper.HTTPServerAddr
	if socks {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				// use dnsgrabber to resolve
				return net.DialTimeout("udp", dnsGrabAddr, 2*time.Second)
			},
		}
		resolved, err := resolver.LookupHost(context.Background(), host)
		log.Debugf("resolved %v to %v: %v", host, resolved, err)
		for _, addr := range resolved {
			ip := net.ParseIP(addr).To4()
			if ip != nil {
				log.Debugf("Using resolved IPv4 address: %v", addr)
				host = addr
				break
			}
		}
	}
	hostWithPort := fmt.Sprintf("%v:80", host)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v/humans.txt", host), nil)
	req.Header.Set("Host", hostWithPort)

	transport := &http.Transport{}
	if socks {
		// Set up SOCKS proxy
		proxyURL, err := url.Parse("socks5://" + proxyAddr)
		if err != nil {
			return fmt.Errorf("Failed to parse proxy URL: %v\n", err)
		}

		socksDialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return fmt.Errorf("Failed to obtain proxy dialer: %v\n", err)
		}
		transport.Dial = socksDialer.Dial
	} else {
		proxyURL, _ := url.Parse("http://" + proxyAddr)
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{
		Timeout:   time.Second * 15,
		Transport: transport,
	}

	var res *http.Response
	var err error

	if res, err = client.Do(req); err != nil {
		return err
	}

	var buf []byte

	buf, err = ioutil.ReadAll(res.Body)

	fmt.Printf(string(buf) + "\n")

	if string(buf) != integrationtest.Content {
		return errors.New("Expecting another response.")
	}

	return nil
}

func TestInternalHeaders(t *testing.T) {
	var tests = []struct {
		input    string
		expected map[string]string
	}{
		// Legit
		{
			"{\"X-Lantern-Foo-Bar\": \"foobar\", \"X-Lantern-Baz\": \"quux\"}",
			map[string]string{"X-Lantern-Foo-Bar": "foobar", "X-Lantern-Baz": "quux"},
		},
		// Ignored
		{
			"",
			map[string]string{},
		},
		{
			"jf91283r7f0--",
			map[string]string{},
		},
		{
			"[\"X-Lantern-Foo-Bar\", \"foobar\"]",
			map[string]string{},
		},
		// Partially ignored
		{
			"{\"X-Lantern-Foo-Bar\": {\"foobar\": \"baz\"}, \"X-Lantern-Baz\": \"quux\"}",
			map[string]string{"X-Lantern-Baz": "quux"},
		},
	}

	for _, test := range tests {
		s := userConfig{testSession{serializedInternalHeaders: test.input}}
		got := s.GetInternalHeaders()
		assert.Equal(t, test.expected, got, "Headers did not decode as expected")
	}
}

// This test requires the tag "lantern" to be set at testing time like:
//
//    go test -tags="lantern"
//
func TestAutoUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skip test in short mode")
	}

	updateCfg := buildUpdateCfg()
	updateCfg.HTTPClient = &http.Client{}
	updateCfg.CurrentVersion = "0.0.1"
	updateCfg.OS = "android"
	updateCfg.Arch = "arm"

	// Update available
	result, err := checkForUpdates(updateCfg)
	require.NoError(t, err)
	assert.Contains(t, result, "update_android_arm.bz2")
	assert.Contains(t, result, strings.ToLower(common.AppName))

	// No update available
	updateCfg.CurrentVersion = "9999.9.9"
	result, err = checkForUpdates(updateCfg)
	require.NoError(t, err)
	assert.Empty(t, result)
}
