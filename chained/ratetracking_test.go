package chained

import (
	"context"
	"sync"
	"testing"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/mockconn"
	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
)

func TestRateTracking(t *testing.T) {
	var mx sync.Mutex
	var finalErr error
	var finalCtx map[string]interface{}
	ops.RegisterReporter(func(failure error, ctx map[string]interface{}) {
		log.Infof("Reporting: %v", ctx)
		mx.Lock()
		val, ok := ctx["client_bytes_sent"].(borda.Val)
		if ok && val.Get() == 8.0 {
			finalErr = failure
			finalCtx = ctx
		}
		mx.Unlock()
	})

	sd := mockconn.SucceedingDialer([]byte("1234567890"))
	p, err := newProxy("test", "proto", "netw", "addr:567", &ChainedServerInfo{
		Addr:      "addr:567",
		AuthToken: "token",
	}, newTestUserConfig(), true, func(ctx context.Context, p *proxy) (serverConn, error) {
		return p.defaultServerConn(sd.Dial("", ""))
	})

	conn, err := p.dial("tcp", "origin:443")
	if !assert.NoError(t, err) {
		return
	}
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

	assert.EqualValues(t, float64(8), finalCtx["client_bytes_sent"])
	assert.True(t, finalCtx["client_bps_sent_min"].(borda.Val).Get() > 0)
	assert.True(t, finalCtx["client_bps_sent_max"].(borda.Val).Get() > 0)
	assert.True(t, finalCtx["client_bps_sent_avg"].(borda.Val).Get() > 0)

	assert.EqualValues(t, float64(10), finalCtx["client_bytes_recv"])
	assert.True(t, finalCtx["client_bps_recv_min"].(borda.Val).Get() > 0)
	assert.True(t, finalCtx["client_bps_recv_max"].(borda.Val).Get() > 0)
	assert.True(t, finalCtx["client_bps_recv_avg"].(borda.Val).Get() > 0)
}
