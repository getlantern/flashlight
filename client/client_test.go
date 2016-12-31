package client

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/balancer"
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

func resetBalancer(dialer func(network, addr string) (net.Conn, error)) {
	bal.Reset(&balancer.Dialer{
		Label:   "test-dialer",
		DialFN:  dialer,
		Check:   func(interface{}, func(string)) (bool, time.Duration) { return true, 10 * time.Millisecond },
		Trusted: true,
	})
}

func TestServeHTTPOk(t *testing.T) {
	mockResponse := []byte("HTTP/1.1 404 Not Found\r\n\r\n")
	client := NewClient(func() bool { return true },
		func() string { return "proToken" })
	d := mockconn.SucceedingDialer(mockResponse)
	resetBalancer(d.Dial)

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
	client := NewClient(func() bool { return true },
		func() string { return "proToken" })
	d := mockconn.SucceedingDialer([]byte{})
	resetBalancer(func(network, addr string) (net.Conn, error) {
		<-time.After(requestTimeout * 2)
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
