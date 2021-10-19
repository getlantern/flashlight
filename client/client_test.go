package client

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/detour"
	"github.com/getlantern/golog"
	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/flashlight/stats"
)

var logger = golog.LoggerFor("client-test")

var tempConfigDir string

func TestMain(m *testing.M) {
	tempConfigDir, err := ioutil.TempDir("", "client_test")
	if err != nil {
		logger.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	os.Exit(m.Run())
}

const (
	testLang            = "en"
	testAdSwapTargetURL = "http://localhost/purchase"
)

type mockStatsTracker struct{}

func (m mockStatsTracker) Latest() stats.Stats                                      { return stats.Stats{} }
func (m mockStatsTracker) AddListener(func(newStats stats.Stats)) (close func())    { return nil }
func (m mockStatsTracker) SetActiveProxyLocation(city, country, countryCode string) {}
func (m mockStatsTracker) IncHTTPSUpgrades()                                        {}
func (m mockStatsTracker) IncAdsBlocked()                                           {}
func (m mockStatsTracker) SetDisconnected(val bool)                                 {}
func (m mockStatsTracker) SetHasSucceedingProxy(val bool)                           {}
func (m mockStatsTracker) SetHitDataCap(val bool)                                   {}
func (m mockStatsTracker) SetIsPro(val bool)                                        {}
func (m mockStatsTracker) SetYinbiEnabled(val bool)                                 {}
func (m mockStatsTracker) SetAlert(stats.AlertType, string, bool)                   {}
func (m mockStatsTracker) ClearAlert(stats.AlertType)                               {}

func init() {
	domainrouting.Configure(nil, &domainrouting.ProxiedSitesConfig{})
}

func newTestUserConfig() *common.UserConfigData {
	return common.NewUserConfigData("device", 1234, "protoken", nil, "en-US")
}

func resetBalancer(client *Client, dialer func(network, addr string) (net.Conn, error)) {
	client.bal.Reset([]balancer.Dialer{&testDialer{
		name: "test-dialer",
		dial: dialer,
	}})
}

func newClient() *Client {
	return newClientWithLangAndAdSwapTargetURL(testLang, testAdSwapTargetURL)
}

func newClientWithLangAndAdSwapTargetURL(lang string, adSwapTargetURL string) *Client {
	client, _ := NewClient(
		tempConfigDir,
		func() bool { return false },
		func() bool { return true },
		func() bool { return false },
		func() bool { return false },
		func(ctx context.Context, addr string) (bool, net.IP) {
			return false, nil
		},
		func() bool { return true },
		func() bool { return true },
		func() bool { return false },
		func() bool { return false },
		newTestUserConfig(),
		mockStatsTracker{},
		func() bool { return true },
		func() string { return lang },
		func() string { return adSwapTargetURL },
		func(host string) (string, error) { return host, nil },
		func() string { return "https://tracker/ads" },
		func(category, action, label string) {},
	)
	return client
}

func TestServeHTTPOk(t *testing.T) {
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	client := newClient()
	resetBalancer(client, mockconn.SucceedingDialer(mockResponse).Dial)

	req, _ := http.NewRequest("CONNECT", "https://b.com:443", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "timeout=58", resp.Header.Get("Keep-Alive"))
	assert.Equal(t, "0", resp.Header.Get("Content-Length"))
}

func TestServeHTTPTimeout(t *testing.T) {
	client := newClient()
	client.requestTimeout = 50 * time.Millisecond
	resetBalancer(client, func(network, addr string) (net.Conn, error) {
		<-time.After(client.requestTimeout * 2)
		return mockconn.SucceedingDialer(nil).Dial(network, addr)
	})

	req, _ := http.NewRequest("CONNECT", "https://a.com:443", nil)
	resp, _ := roundTrip(client, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "CONNECT requests should always succeed")

	req, _ = http.NewRequest("GET", "http://b.com/action", nil)
	req.Header.Set("Accept", "text/html")
	resp, _ = roundTrip(client, req)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "It should respond 503 Service Unavailable with error page")
	body, err := ioutil.ReadAll(resp.Body)
	if !assert.NoError(t, err) {
		return
	}
	assert.Contains(t, string(body), "<title>Lantern: Error Accessing Page</title>", "should respond with error page")
	assert.Contains(t, string(body), "Still unable to dial", "should be dial error")
}

func TestIsAddressProxyable(t *testing.T) {
	client := newClient()
	assert.NoError(t, client.isAddressProxyable("192.168.1.1:9999"),
		"all addresses should be proxyable when allow private hosts")
	assert.NoError(t, client.isAddressProxyable("localhost:80"),
		"all addresses should be proxyable when allow private hosts")
	client.allowPrivateHosts = func() bool {
		return false
	}
	assert.Error(t, client.isAddressProxyable("192.168.1.1:9999"),
		"private address should not be proxyable")
	assert.Error(t, client.isAddressProxyable("192.168.1.1"),
		"address without port should not be proxyable")
	// Note that in reality, browser / OS may choose to never proxy localhost
	// URLs.
	assert.Error(t, client.isAddressProxyable("www.google.com"),
		"address should not be proxyable if it's missing a port")
	assert.Error(t, client.isAddressProxyable("localhost:80"),
		"address should not be proxyable if it's a plain hostname")
	assert.Error(t, client.isAddressProxyable("localhost"),
		"address should not be proxyable if it's a plain hostname")
	assert.Error(t, client.isAddressProxyable("plainhostname:80"),
		"address should not be proxyable if it's a plain hostname")
	assert.Error(t, client.isAddressProxyable("something.local:80"),
		"address should not be proxyable if it ends in .local")
	assert.Error(t, client.isAddressProxyable("something.onion:80"),
		"address should not be proxyable if it ends in .onion")
	assert.NoError(t, client.isAddressProxyable("anysite.com:80"),
		"address should be proxyable if it's not an IP address, not a plain hostname and does not end in .local")
}

func TestDialShortcut(t *testing.T) {
	site := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("abc"))
		}),
	)
	addr := site.Listener.Addr().String()
	_, p, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(p)
	proxiedCONNECTPorts = append(proxiedCONNECTPorts, port)
	client := newClient()
	shortcutVisited := 0

	client.allowShortcutTo = func(ctx context.Context, addr string) (bool, net.IP) {
		shortcutVisited++
		return true, net.ParseIP(addr)
	}

	req, _ := http.NewRequest("GET", site.URL, nil)
	res, _ := roundTrip(client, req)
	assert.Zero(t, shortcutVisited, "should not check shortcut list when shortcut is disabled")

	client.useShortcut = func() bool { return true }
	// used as a sign that the request is sent to proxy
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	// add some delay before sending back data, as response before the request
	// was sent is apparently not expected by http client, which would cause
	// http.Transport to print "Unsolicited response received on idle HTTP
	// channel..." and return readLoopPeekFailLocked error.
	delayed404 := mockconn.SlowResponder(mockconn.SucceedingDialer(mockResponse), 50*time.Millisecond)
	resetBalancer(client, delayed404.Dial)

	req, _ = http.NewRequest("GET", site.URL, nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 1, shortcutVisited, "should check shortcut list")
	assert.Equal(t, 200, res.StatusCode, "should respond with 200 when a shortcutted site is reachable")
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, "abc", string(body), "should respond with correct content")

	req, _ = http.NewRequest("GET", "http://unknown:80", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 2, shortcutVisited, "should check shortcut list")
	assert.Equal(t, 404, res.StatusCode, "should dial proxy if the shortcutted site is unreachable")

	req, _ = http.NewRequest("CONNECT", "http://unknown2:80", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 3, shortcutVisited, "should check shortcut list")
	nestedResp, err := res.nested()
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 404, nestedResp.StatusCode, "should dial proxy if the shortcutted site is unreachable")

	client.useShortcut = func() bool { return true }
	client.allowShortcutTo = func(context.Context, string) (bool, net.IP) {
		shortcutVisited++
		return false, nil
	}
	req, _ = http.NewRequest("CONNECT", "http://unknown3:80", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 4, shortcutVisited, "should check shortcut list")
	nestedResp, err = res.nested()
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 404, nestedResp.StatusCode, "should dial proxy if the site is not shortcutted")

	detour.AddToWl("unknown4:80", true)
	defer detour.RemoveFromWl("unknown4:80")
	req, _ = http.NewRequest("GET", "http://unknown4:80", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 4, shortcutVisited, "should not check shortcut list if the site is whitelisted")
	assert.Equal(t, 404, res.StatusCode, "should dial proxy if the site is whitelisted")

	detour.AddToWl("unknown5:80", true)
	defer detour.RemoveFromWl("unknown5:80")
	req, _ = http.NewRequest("CONNECT", "http://unknown5:80", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 4, shortcutVisited, "should not check shortcut list if the site is whitelisted")
	nestedResp, err = res.nested()
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 404, nestedResp.StatusCode, "should dial proxy if the site is whitelisted")
}

// See https://github.com/getlantern/lantern-internal/issues/4267
func TestLeakingDomainsRequiringProxy(t *testing.T) {
	domainrouting.Configure(nil, &domainrouting.ProxiedSitesConfig{
		Cloud: []string{"proxy.me"},
	})
	defer domainrouting.Configure(nil, &domainrouting.ProxiedSitesConfig{})

	site := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("abc"))
		}),
	)
	addr := site.Listener.Addr().String()
	_, p, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(p)
	proxiedCONNECTPorts = append(proxiedCONNECTPorts, port)
	client := newClient()

	client.allowShortcutTo = func(ctx context.Context, addr string) (bool, net.IP) {
		return false, nil
	}
	client.useShortcut = func() bool { return false }

	detour.AddToWl(addr, true)
	defer detour.RemoveFromWl(site.URL)

	// used as a sign that the request is sent to proxy

	mockResponse := []byte("HTTP/1.1 418 I'm a Teapot\r\n\r\n")
	// add some delay before sending back data, as response before the request
	// was sent is apparently not expected by http client, which would cause
	// http.Transport to print "Unsolicited response received on idle HTTP
	// channel..." and return readLoopPeekFailLocked error.
	delayed418 := mockconn.SlowResponder(mockconn.SucceedingDialer(mockResponse), 50*time.Millisecond)
	resetBalancer(client, delayed418.Dial)

	req, _ := http.NewRequest("GET", "http://getiantem.org", nil)
	res, _ := roundTrip(client, req)
	assert.Equal(t, 418, res.StatusCode, "should dial proxy for domain requiring proxying")

	req, _ = http.NewRequest("GET", "http://proxy.me", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 418, res.StatusCode, "should dial proxy for domain with proxy domainrouting rule")

	req, _ = http.NewRequest("GET", site.URL, nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 418, res.StatusCode, "should dial via proxy for random site if client is connected")

	// setting disconnected status
	client.disconnected = func() bool { return true }

	req, _ = http.NewRequest("GET", site.URL, nil)
	res, _ = roundTrip(client, req)
	body, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode, "should dial directly for random site if client is disconnected")
	assert.Equal(t, "abc", string(body), "should dial directly for random site if client is disconnected")

	req, _ = http.NewRequest("GET", "http://getiantem.org", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 418, res.StatusCode, "should dial proxy for domain requiring proxying even if client is disconnected")

	req, _ = http.NewRequest("GET", "http://subdomain.getiantem.org", nil)
	res, _ = roundTrip(client, req)
	assert.Equal(t, 418, res.StatusCode, "should dial proxy for subdomain requiring proxying even if client is disconnected")

	domainrouting.Configure(nil, &domainrouting.ProxiedSitesConfig{
		Cloud: []string{"localhost"},
	})

	req, _ = http.NewRequest("GET", site.URL, nil)
	res, _ = roundTrip(client, req)
	body, err = ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode, "should dial directly for domain with proxy domainrouting rule when client is disconnected")
	assert.Equal(t, "abc", string(body), "should dial directly for domain with proxy domainrouting rule when client is disconnected")

}

// See https://github.com/getlantern/lantern-internal/issues/2724
func TestTimeoutCheckingShortcut(t *testing.T) {
	requestTimeout := 10 * time.Millisecond
	client := newClient()
	client.requestTimeout = requestTimeout
	client.useDetour = func() bool { return false }
	client.useShortcut = func() bool { return true }
	client.allowShortcutTo = func(ctx context.Context, addr string) (bool, net.IP) {
		// In theory allowShortcut should never exceed the context deadline,
		// but it could happen in cases like computer resuming from sleeping.
		time.Sleep(requestTimeout * 2)
		return false, nil
	}

	dialer := mockconn.SucceedingDialer([]byte("whatever"))
	resetBalancer(client, dialer.Dial)
	req, _ := http.NewRequest("GET", "http://unknown:80", nil)
	res, _ := roundTrip(client, req)
	assert.Equal(t, 503, res.StatusCode, "should fail if checking shortcut list times out")
	assert.True(t, dialer.AllClosed(), "should not dial proxy if checking shourcut list times out")
}

func TestAccessingProxyPort(t *testing.T) {
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	client := newClient()
	resetBalancer(client, mockconn.SucceedingDialer(mockResponse).Dial)

	go func() {
		client.ListenAndServeHTTP("localhost:", func() {
		})
	}()
	listenAddr, valid := Addr(24 * time.Hour)
	assert.True(t, valid, "should set addr")
	proxyURL := "http://" + listenAddr.(string)
	tr := http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return url.Parse(proxyURL)
		},
	}

	req, _ := http.NewRequest("GET", proxyURL, nil)
	resp, err := tr.RoundTrip(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get("Content-Length"))

	_, port, _ := net.SplitHostPort(listenAddr.(string))
	req, _ = http.NewRequest("GET", "http://localhost:"+port, nil)
	resp, err = tr.RoundTrip(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get("Content-Length"))
}

type testDialer struct {
	name      string
	rtt       time.Duration
	dial      func(network, addr string) (net.Conn, error)
	bandwidth float64
	untrusted bool
	failing   bool
	attempts  int64
	successes int64
	failures  int64
	stopped   bool
}

// Name returns the name for this Dialer
func (d *testDialer) Name() string {
	return d.name
}

func (d *testDialer) Label() string {
	return d.name
}

func (d *testDialer) JustifiedLabel() string {
	return d.name
}

func (d *testDialer) Location() (string, string, string) {
	return "", "", ""
}

func (d *testDialer) Protocol() string {
	return "https"
}

func (d *testDialer) Addr() string {
	return ""
}

func (d *testDialer) Trusted() bool {
	return !d.untrusted
}

func (d *testDialer) NumPreconnecting() int {
	return 0
}

func (d *testDialer) NumPreconnected() int {
	return 0
}

func (d *testDialer) Dial(network, addr string) (net.Conn, error) {
	conn, _, err := d.DialContext(context.Background(), network, addr)
	return conn, err
}

func (d *testDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	var conn net.Conn
	var err error
	if !d.Succeeding() {
		err = fmt.Errorf("Failing intentionally")
	} else if network != "" {
		chDone := make(chan bool)
		go func() {
			conn, err = d.dial(network, addr)
			chDone <- true
		}()
		select {
		case <-chDone:
			return conn, true, err
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	atomic.AddInt64(&d.attempts, 1)
	if err == nil {
		atomic.AddInt64(&d.successes, 1)
	} else {
		atomic.AddInt64(&d.failures, 1)
	}
	return conn, false, err
}

func (d *testDialer) MarkFailure() {
	atomic.AddInt64(&d.failures, 1)
}

func (d *testDialer) EstRTT() time.Duration {
	return d.rtt
}

func (d *testDialer) EstBandwidth() float64 {
	return d.bandwidth
}

func (d *testDialer) EstSuccessRate() float64 {
	return 0
}

func (d *testDialer) ProbeStats() (successes uint64, successKBs uint64, failures uint64, failedKBs uint64) {
	return 0, 0, 0, 0
}

func (d *testDialer) Attempts() int64 {
	return atomic.LoadInt64(&d.attempts)
}

func (d *testDialer) Successes() int64 {
	return atomic.LoadInt64(&d.successes)
}

func (d *testDialer) ConsecSuccesses() int64 {
	return 0
}

func (d *testDialer) Failures() int64 {
	return atomic.LoadInt64(&d.failures)
}

func (d *testDialer) ConsecFailures() int64 {
	return 0
}

func (d *testDialer) Succeeding() bool {
	return !d.failing
}

func (d *testDialer) DataRecv() uint64 {
	return 0
}

func (d *testDialer) DataSent() uint64 {
	return 0
}

func (d *testDialer) CheckConnectivity() bool {
	return true
}

func (d *testDialer) Probe(forPerformance bool) bool {
	return true
}

func (d *testDialer) Stop() {
	d.stopped = true
}

func (d *testDialer) Ping()                {}
func (d *testDialer) WriteStats(io.Writer) {}

func roundTrip(client *Client, req *http.Request) (*response, error) {
	toSend := &bytes.Buffer{}
	err := req.Write(toSend)
	if err != nil {
		return nil, err
	}
	received := &bytes.Buffer{}
	err = client.handle(mockconn.New(received, toSend))
	if err != nil {
		logger.Errorf("Error handling: %v", err)
	}
	br := bufio.NewReader(bytes.NewReader(received.Bytes()))
	resp, err2 := http.ReadResponse(br, req)
	if err == nil {
		err = err2
	}
	return &response{*resp, req, br}, err
}

type response struct {
	http.Response
	req *http.Request
	br  *bufio.Reader
}

func (r *response) nested() (*http.Response, error) {
	return http.ReadResponse(r.br, r.req)
}
