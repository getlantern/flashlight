package balancer

import (
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func TestRateTracking(t *testing.T) {
	var mx sync.Mutex
	var finalErr error
	var finalCtx map[string]interface{}
	ops.RegisterReporter(func(failure error, ctx map[string]interface{}) {
		mx.Lock()
		if ctx["metric_client_bytes_sent"] == 8.0 {
			finalErr = failure
			finalCtx = ctx
		}
		mx.Unlock()
	})

	sd := mockconn.SucceedingDialer([]byte("1234567890"))
	wrapped, err := sd.Dial("", "")
	if !assert.NoError(t, err) {
		return
	}
	conn := withRateTracking(wrapped, "origin:443", nil)
	n, err := conn.Write([]byte("12345678"))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 8, n)
	b := make([]byte, 1000)
	n, err = conn.Read(b)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 10, n)
	// Be inactive for a bit
	time.Sleep(3 * rateInterval)
	conn.Close()

	// Wait for tracking to finish
	time.Sleep(2 * rateInterval)

	mx.Lock()
	defer mx.Unlock()
	assert.Nil(t, finalErr)
	if !assert.NotNil(t, finalCtx) {
		return
	}

	assert.Equal(t, "xfer", finalCtx["op"])

	assert.Equal(t, float64(8), finalCtx["metric_client_bytes_sent"])
	assert.True(t, finalCtx["metric_client_bps_sent_min"].(float64) > 0)
	assert.True(t, finalCtx["metric_client_bps_sent_max"].(float64) > 0)
	assert.True(t, finalCtx["metric_client_bps_sent_avg"].(float64) > 0)

	assert.Equal(t, float64(10), finalCtx["metric_client_bytes_recv"])
	assert.True(t, finalCtx["metric_client_bps_recv_min"].(float64) > 0)
	assert.True(t, finalCtx["metric_client_bps_recv_max"].(float64) > 0)
	assert.True(t, finalCtx["metric_client_bps_recv_avg"].(float64) > 0)
}
