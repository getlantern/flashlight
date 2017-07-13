package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/detour"
	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"
)

const (
	testLang            = "en"
	testAdSwapTargetURL = "http://localhost/purchase"
)

func newMockWriter() *mockWriter {
	return &mockWriter{
		ResponseWriter: httptest.NewRecorder(),
		Dialer:         mockconn.SucceedingDialer(nil),
	}
}

type mockWriter struct {
	// client.ServeHTTP requires the interface but never used, as the
	// connection is always hijacked
	http.ResponseWriter
	http.Hijacker
	mockconn.Dialer
}

func (w *mockWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, err := w.Dialer.Dial("net", "hijacked")
	return conn,
		bufio.NewReadWriter(
			bufio.NewReader(bytes.NewBuffer(nil)), // don't read request anyway
			bufio.NewWriter(conn)),
		err
}

func (w *mockWriter) ReadResponse() (*http.Response, error) {
	return http.ReadResponse(bufio.NewReader(bytes.NewReader(w.Dialer.Received())), nil)
}

func (w *mockWriter) ReadTunneledResponse() (*http.Response, error) {
	r := bufio.NewReader(bytes.NewReader(w.Dialer.Received()))
	_, _ = http.ReadResponse(r, nil)
	return http.ReadResponse(r, nil)
}

type mockStatsTracker struct{}

func (m mockStatsTracker) SetActiveProxyLocation(city, country, countryCode string) {}
func (m mockStatsTracker) IncHTTPSUpgrades()                                        {}
func (m mockStatsTracker) IncAdsBlocked()                                           {}

func resetBalancer(client *Client, dialer func(network, addr string) (net.Conn, error)) {
	client.bal.Reset(&testDialer{
		name: "test-dialer",
		dial: dialer,
	})
}

func newClient() *Client {
	client, _ := NewClient(
		func(addr string) (bool, net.IP) { return false, nil },
		func() bool { return true },
		func() string { return "proToken" },
		mockStatsTracker{},
		func() bool { return true },
		func() string { return testLang },
		func() string { return testAdSwapTargetURL },
	)
	return client
}

func TestServeHTTPOk(t *testing.T) {
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	client := newClient()
	resetBalancer(client, mockconn.SucceedingDialer(mockResponse).Dial)

	w := newMockWriter()
	req, _ := http.NewRequest("CONNECT", "https://b.com:443", nil)
	client.ServeHTTP(w, req)
	assert.Equal(t, "hijacked", w.Dialer.LastDialed())
	res, _ := w.ReadTunneledResponse()
	assert.Equal(t, 404, res.StatusCode, "CONNECT requests should get 404 Not Found in tunnel")

	// disable the test temporarily. It has weird error "readLoopPeekFailLocked <nil>" when run with `go test -race`
	// w = newMockWriter()
	// req, _ = http.NewRequest("GET", "http://a.com/page.html", nil)
	// req.Header.Set("Accept", "not-html")
	// client.ServeHTTP(w, req)
	// assert.Equal(t, "hijacked", w.Dialer.LastDialed())
	// res, _ = w.ReadResponse()
	// assert.Equal(t, 404, res.StatusCode, "non-CONNECT requests should get 404 Not Found")
}

func TestServeHTTPTimeout(t *testing.T) {
	originalRequestTimeout := getRequestTimeout()
	atomic.StoreInt64(&requestTimeout, int64(50*time.Millisecond))
	defer func() {
		atomic.StoreInt64(&requestTimeout, int64(originalRequestTimeout))
	}()

	client := newClient()
	resetBalancer(client, func(network, addr string) (net.Conn, error) {
		<-time.After(getRequestTimeout() * 2)
		return mockconn.SucceedingDialer(nil).Dial(network, addr)
	})

	w := newMockWriter()
	req, _ := http.NewRequest("CONNECT", "https://a.com:443", nil)
	client.ServeHTTP(w, req)
	res, _ := w.ReadResponse()
	assert.Equal(t, 200, res.StatusCode, "CONNECT requests should still succeed")

	w = newMockWriter()
	req, _ = http.NewRequest("GET", "http://b.com/action", nil)
	req.Header.Set("Accept", "text/html")
	client.ServeHTTP(w, req)
	res, _ = w.ReadResponse()
	assert.Equal(t, 503, res.StatusCode, "non-CONNECT requests should get 503 response")
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "<title>Lantern: Error Accessing Page</title>", "should respond with error page")
	assert.Contains(t, string(body), "context deadline exceeded", "should include the detailed error")
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
	shortcutVisited := false
	client.allowShortcut = func(addr string) (bool, net.IP) {
		shortcutVisited = true
		return true, net.ParseIP(addr)
	}
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n") // used as a sign that the request is sent to proxy
	resetBalancer(client, mockconn.SucceedingDialer(mockResponse).Dial)

	w := newMockWriter()
	req, _ := http.NewRequest("GET", site.URL, nil)
	client.ServeHTTP(w, req)
	assert.True(t, shortcutVisited)
	assert.Equal(t, "hijacked", w.Dialer.LastDialed())
	res, _ := w.ReadResponse()
	assert.Equal(t, 200, res.StatusCode, "should respond with 200 when a shortcutted site is reachable")
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, "abc", string(body), "should respond with correct content")

	// disable the test temporarily. It has weird error "readLoopPeekFailLocked <nil>" when run with `go test -race`
	// w = newMockWriter()
	// req, _ = http.NewRequest("GET", "http://unknown:80", nil)
	// shortcutVisited = false
	// client.ServeHTTP(w, req)
	// assert.True(t, shortcutVisited)
	// res, _ = w.ReadResponse()
	// assert.Equal(t, 404, res.StatusCode, "should dial proxy if the shortcutted site is unreachable")

	w = newMockWriter()
	req, _ = http.NewRequest("CONNECT", "http://unknown2:80", nil)
	shortcutVisited = false
	client.ServeHTTP(w, req)
	assert.True(t, shortcutVisited)
	res, _ = w.ReadTunneledResponse()
	assert.Equal(t, 404, res.StatusCode, "should dial proxy if the shortcutted site is unreachable")

	client.allowShortcut = func(addr string) (bool, net.IP) {
		shortcutVisited = true
		return false, nil
	}
	w = newMockWriter()
	req, _ = http.NewRequest("CONNECT", "http://unknown3:80", nil)
	shortcutVisited = false
	client.ServeHTTP(w, req)
	assert.True(t, shortcutVisited)
	res, _ = w.ReadTunneledResponse()
	assert.Equal(t, 404, res.StatusCode, "should dial proxy if the site is not shortcutted")

	// disable the test temporarily. It has weird error "readLoopPeekFailLocked <nil>" when run with `go test -race`
	// detour.AddToWl("unknown4:80", true)
	// defer detour.RemoveFromWl("unknown4:80")
	// w = newMockWriter()
	// req, _ = http.NewRequest("GET", "http://unknown4:80", nil)
	// shortcutVisited = false
	// client.ServeHTTP(w, req)
	// assert.False(t, shortcutVisited, "should not check shortcut list if the site is whitelisted")
	// res, _ = w.ReadResponse()
	// assert.Equal(t, 404, res.StatusCode, "should dial proxy if the site is whitelisted")

	detour.AddToWl("unknown5:80", true)
	defer detour.RemoveFromWl("unknown5:80")
	w = newMockWriter()
	req, _ = http.NewRequest("CONNECT", "http://unknown5:80", nil)
	shortcutVisited = false
	client.ServeHTTP(w, req)
	assert.False(t, shortcutVisited, "should not check shortcut list if the site is whitelisted")
	res, _ = w.ReadTunneledResponse()
	assert.Equal(t, 404, res.StatusCode, "should dial proxy if the site is whitelisted")

}

type testDialer struct {
	name      string
	latency   time.Duration
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

func (d *testDialer) Addr() string {
	return ""
}

func (d *testDialer) Trusted() bool {
	return !d.untrusted
}

func (d *testDialer) Dial(network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error
	if !d.Succeeding() {
		err = fmt.Errorf("Failing intentionally")
	} else if network != "" {
		conn, err = d.dial(network, addr)
	}
	atomic.AddInt64(&d.attempts, 1)
	if err == nil {
		atomic.AddInt64(&d.successes, 1)
	} else {
		atomic.AddInt64(&d.failures, 1)
	}
	return conn, err
}

func (d *testDialer) EMADialTime() time.Duration {
	return 0
}

func (d *testDialer) EstLatency() time.Duration {
	return d.latency
}

func (d *testDialer) EstBandwidth() float64 {
	return d.bandwidth
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

func (d *testDialer) CheckConnectivity() bool {
	return true
}

func (d *testDialer) ProbePerformance() {
}

func (d *testDialer) Stop() {
	d.stopped = true
}
