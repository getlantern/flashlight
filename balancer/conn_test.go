package balancer

import (
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConnMetrics(t *testing.T) {
	var finalErr error
	var finalCtx map[string]interface{}
	ops.RegisterReporter(func(failure error, ctx map[string]interface{}) {
		finalErr = failure
		finalCtx = ctx
	})

	sd := mockconn.SucceedingDialer([]byte("1234567890"))
	wrapped, err := sd.Dial("", "")
	if !assert.NoError(t, err) {
		return
	}
	conn := wrap(wrapped, "origin:443", nil)
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
	conn.Close()

	assert.Nil(t, finalErr)
	if !assert.NotNil(t, finalCtx) {
		return
	}

	assert.Equal(t, "xfer", finalCtx["op"])
	assert.Equal(t, float64(8), finalCtx["client_bytes_sent"])
	assert.Equal(t, float64(10), finalCtx["client_bytes_recv"])
	assert.True(t, finalCtx["client_conn_bytes_sent_per_second"].(float64) > 0)
	assert.True(t, finalCtx["client_conn_bytes_recv_per_second"].(float64) > 0)
}
