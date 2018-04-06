package android

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/stretchr/testify/assert"
)

const expectedBody = "Google is built by a large team of engineers, designers, researchers, robots, and others in many different sites across the globe. It is updated continuously, and built with more tools and technologies than we can shake a stick at. If you'd like to help us out, see google.com/careers.\n"

type testProtector struct{}

type testSession struct {
	Session
}

type testSettings struct {
	Settings
}

func (c testSettings) StickyConfig() bool       { return false }
func (c testSettings) EnableAdBlocking() bool   { return false }
func (c testSettings) DefaultDnsServer() string { return "8.8.8.8" }
func (c testSettings) TimeoutMillis() int       { return 15000 }
func (c testSettings) GetHttpProxyHost() string { return "127.0.0.1" }
func (c testSettings) GetHttpProxyPort() int    { return 49128 }

func (c testSession) AfterStart()                   {}
func (c testSession) BandwidthUpdate(int, int, int) {}
func (c testSession) ConfigUpdate(bool)             {}
func (c testSession) ShowSurvey(survey string)      {}
func (c testSession) GetUserID() int64              { return 0 }
func (c testSession) GetToken() string              { return "" }
func (c testSession) GetDNSServer() string          { return "8.8.8.8" }
func (c testSession) SetStaging(bool)               {}
func (c testSession) SetCountry(string)             {}
func (c testSession) ProxyAll() bool                { return true }
func (c testSession) GetDeviceID() string           { return "123456789" }
func (c testSession) AccountId() string             { return "1234" }
func (c testSession) Locale() string                { return "en-US" }
func (c testSession) SetUserId(int64)               {}
func (c testSession) SetToken(string)               {}
func (c testSession) SetCode(string)                {}
func (c testSession) IsProUser() bool               { return true }
func (c testSession) AdBlockingAllowed() bool       { return false }

func (c testSession) UpdateStats(string, string, string, int, int) {}

func (c testSession) UpdateAdSettings(AdSettings) {}

func (c testSession) SerializedInternalHeaders() string { return "" }

func TestProxying(t *testing.T) {

	tmpDir, err := ioutil.TempDir("", "testconfig")
	if assert.NoError(t, err, "Unable to create temp configDir") {
		defer os.RemoveAll(tmpDir)
		result, err := Start(tmpDir, "en_US", testSettings{}, testSession{})
		if assert.NoError(t, err, "Should have been able to start lantern") {
			newResult, err := Start("testapp", "en_US", testSettings{}, testSession{})
			if assert.NoError(t, err, "Should have been able to start lantern twice") {
				if assert.Equal(t, result.HTTPAddr, newResult.HTTPAddr, "2nd start should have resulted in the same address") {
					err := testProxiedRequest(result.HTTPAddr, false)
					if assert.NoError(t, err, "Proxying request via HTTP should have worked") {
						err := testProxiedRequest(result.SOCKS5Addr, true)
						assert.NoError(t, err, "Proxying request via SOCKS should have worked")
					}
				}
			}
		}
	}
}

func testProxiedRequest(proxyAddr string, socks bool) error {
	host := "www.google.com"
	if socks {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				// use dnsgrabber to resolve
				return net.DialTimeout("udp", "127.0.0.1:8153", 2*time.Second)
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

	fmt.Printf(string(buf))

	if string(buf) != expectedBody {
		return errors.New("Expecting another response.")
	}

	return nil
}
