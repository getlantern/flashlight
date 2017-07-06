package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"
)

const (
	testLang            = "en"
	testAdSwapTargetURL = "http://localhost/purchase"
)

type mockWriter struct {
	http.ResponseWriter
	http.Hijacker
	mockconn.Dialer
}

func (w mockWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	log.Debug("Hijacking")
	conn, err := w.Dialer.Dial("net", "hijacked")
	return conn,
		bufio.NewReadWriter(
			bufio.NewReader(bytes.NewBuffer(nil)), // don't read request anyway
			bufio.NewWriter(conn)),
		err
}

func (w mockWriter) Status() int {
	return w.ResponseWriter.(*httptest.ResponseRecorder).Code
}

func (w mockWriter) Dump() string {
	return fmt.Sprintf("%+v", *w.ResponseWriter.(*httptest.ResponseRecorder).Result())
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
		func() bool { return true },
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
	d := mockconn.SucceedingDialer(mockResponse)
	resetBalancer(client, d.Dial)

	req, _ := http.NewRequest("CONNECT", "https://b.com:443", nil)
	resp, err := roundTrip(client, req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "timeout=38", resp.Header.Get("Keep-Alive"))
	assert.Equal(t, "0", resp.Header.Get("Content-Length"))
}

func TestServeHTTPTimeout(t *testing.T) {
	originalRequestTimeout := getRequestTimeout()
	atomic.StoreInt64(&requestTimeout, int64(50*time.Millisecond))
	defer func() {
		atomic.StoreInt64(&requestTimeout, int64(originalRequestTimeout))
	}()

	client := newClient()
	d := mockconn.SucceedingDialer([]byte{})
	resetBalancer(client, func(network, addr string) (net.Conn, error) {
		<-time.After(getRequestTimeout() * 2)
		return d.Dial(network, addr)
	})

	req, _ := http.NewRequest("CONNECT", "https://a.com:443", nil)
	resp, _ := roundTrip(client, req)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "CONNECT requests should always succeed")

	req, _ = http.NewRequest("GET", "http://b.com/action", nil)
	req.Header.Set("Accept", "not-html")
	resp, _ = roundTrip(client, req)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "It should respond 503 Service Unavailable with error page")
	body, err := ioutil.ReadAll(resp.Body)
	if !assert.NoError(t, err) {
		return
	}
	assert.Contains(t, string(body), "context deadline exceeded", "should be with context error")
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

func roundTrip(client *Client, req *http.Request) (*http.Response, error) {
	toSend := &bytes.Buffer{}
	err := req.Write(toSend)
	if err != nil {
		return nil, err
	}
	received := &bytes.Buffer{}
	err = client.handle(mockconn.New(received, toSend))
	if err != nil {
		log.Errorf("Error handling: %v", err)
	}
	resp, err2 := http.ReadResponse(bufio.NewReader(bytes.NewReader(received.Bytes())), req)
	if err == nil {
		err = err2
	}
	return resp, err
}
