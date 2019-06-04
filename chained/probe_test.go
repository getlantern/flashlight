package chained

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/common"
)

func TestProbe(t *testing.T) {
	for _, forPerformance := range []bool{false, true} {
		var requests []*http.Request
		server := httptest.NewServer(
			http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				requests = append(requests, req)
			}))

		proxy, err := newHTTPProxy("test-proxy", "http", "tcp", &ChainedServerInfo{
			Addr: server.Listener.Addr().String(),
		}, &common.UserConfigData{})
		assert.NoError(t, err)

		assert.True(t, proxy.Probe(forPerformance))
		for _, req := range requests {
			assert.Equal(t, "ping-chained-server", req.Host)
		}
		if forPerformance {
			assert.Equal(t, PerformanceProbes, len(requests))
			assert.Equal(t, "clear", requests[0].Header.Get("X-BBR"),
				"Probing for performance should ask the proxy to clear BBR info")
			for i, req := range requests {
				assert.Equal(t, strconv.Itoa(BasePerformanceProbeKB+i*25),
					req.Header.Get(common.PingHeader))
			}
		} else {
			assert.Equal(t, Probes, len(requests))
			for _, req := range requests {
				assert.Equal(t, "1", req.Header.Get(common.PingHeader))
			}
		}
	}
}

func TestProbeFailing(t *testing.T) {
	uc := &common.UserConfigData{}
	addr := "localhost:1"
	proxy, err := newHTTPProxy("test-proxy", "http", "tcp", &ChainedServerInfo{Addr: addr}, uc)
	assert.NoError(t, err)
	assert.False(t, proxy.Probe(false),
		"testing against non-existent port should have failed")

	server := httptest.NewUnstartedServer(
		http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			rw.WriteHeader(http.StatusServiceUnavailable)
		}))
	addr = server.Listener.Addr().String()
	proxy, err = newHTTPProxy("test-proxy", "http", "tcp", &ChainedServerInfo{Addr: addr}, uc)
	assert.NoError(t, err)
	// Disable below to avoid wasting 20s in CI
	// assert.False(t, proxy.Probe(false),
	// 	"testing against unresponsive proxy should have failed")

	server.Start()
	assert.False(t, proxy.Probe(false),
		"Unexpected HTTP status code should be treated as fail")
}
