package client

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"
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
	client, _ := NewClient(func() bool { return true },
		func() string { return "proToken" },
		mockStatsTracker{},
		func() bool { return true },
	)
	return client
}

func TestServeHTTPOk(t *testing.T) {
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	client := newClient()
	d := mockconn.SucceedingDialer(mockResponse)
	resetBalancer(client, d.Dial)

	w := mockWriter{ResponseWriter: httptest.NewRecorder(), Dialer: mockconn.SucceedingDialer(mockResponse)}
	req, _ := http.NewRequest("CONNECT", "https://b.com:443", nil)
	client.ServeHTTP(w, req)
	assert.Equal(t, "hijacked", w.Dialer.LastDialed())
	assert.Equal(t, "HTTP/1.1 200 OK\r\nKeep-Alive: timeout=38\r\nContent-Length: 0\r\n\r\nHTTP/1.1 404 Not Found\r\n\r\n", string(w.Dialer.Received()))

	// disable the test temporarily. It has weird error "readLoopPeekFailLocked <nil>" when run with `go test -race`
	/*w = mockWriter{ResponseWriter: httptest.NewRecorder(), Dialer: mockconn.SucceedingDialer([]byte{})}
	req, _ = http.NewRequest("GET", "http://a.com/page.html", nil)
	req.Header.Set("Accept", "not-html")
	client.ServeHTTP(w, req)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, "a.com:80", d.LastDialed())
	assert.Contains(t, string(w.Dialer.Received()), "HTTP/1.1 404 Not Found")*/
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

	w := mockWriter{ResponseWriter: httptest.NewRecorder(), Dialer: d}
	req, _ := http.NewRequest("CONNECT", "https://a.com:443", nil)
	client.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Status(), "CONNECT requests should always succeed")

	w = mockWriter{ResponseWriter: httptest.NewRecorder(), Dialer: d}
	req, _ = http.NewRequest("GET", "http://b.com/action", nil)
	req.Header.Set("Accept", "not-html")
	client.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Status(), "It should respond 200 OK with error page")
	assert.Contains(t, string(w.Dialer.Received()), "context deadline exceeded", "should be with context error")
}

func TestIsAddressProxyable(t *testing.T) {
	client := newClient()
	assert.NoError(t, client.isAddressProxyable("192.168.1.1:9999"),
		"all addresses should be proxyable when allow private hosts")
	assert.NoError(t, client.isAddressProxyable("localhost:80"),
		"all addresses should be proxyable when allow private hosts")
	client.allowPrivateHosts = false
	assert.Error(t, client.isAddressProxyable("192.168.1.1:9999"),
		"private address should not be proxyable")
	assert.Error(t, client.isAddressProxyable("192.168.1.1"),
		"address without port should not be proxyable")
	// Note that in reality, browser / OS may choose to never proxy localhost
	// URLs.
	assert.NoError(t, client.isAddressProxyable("localhost:80"),
		"address should be proxyable if it's not an IP address")
	assert.NoError(t, client.isAddressProxyable("anysite.com:80"),
		"address should be proxyable if it's not an IP address")
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

func (d *testDialer) ProbePerformance() {
}

func (d *testDialer) Stop() {
	d.stopped = true
}
