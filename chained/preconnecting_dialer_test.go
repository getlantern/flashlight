package chained

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	maxSuccessfulPreconnects = 10
	maxSuccessfulDials       = 20
)

var emptyConn net.Conn

type testConn struct {
	net.Conn
	id string
}

func (conn *testConn) Close() error {
	return nil
}

func newEmptyDialer() proxyImpl {
	iod := int64(0)
	ipc := int64(0)

	return &testImpl{d: func(ctx context.Context) (net.Conn, error) {
		var id string
		if ctx == nil {
			if atomic.LoadInt64(&iod) >= maxSuccessfulDials {
				return nil, errors.New("Failing to dial")
			}
			id = fmt.Sprintf("ondemand-%d", atomic.AddInt64(&iod, 1))
		} else {
			if atomic.LoadInt64(&ipc) >= maxSuccessfulPreconnects {
				return nil, errors.New("Failing to preconnect")
			}
			id = fmt.Sprintf("preconnected-%d", atomic.AddInt64(&ipc, 1))
		}
		return &testConn{emptyConn, id}, nil
	}}
}

func TestPreconnecting(t *testing.T) {
	pd := newPreconnectingDialer("testPreconnecting", 2, 10*time.Second, newEmptyDialer())
	defer pd.close()

	conn, err := pd.dialServer(nil, nil)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "ondemand-1", conn.(*testConn).id, "Should have gotten on demand dialed connection")

	for i := 1; i <= maxSuccessfulPreconnects; i++ {
		time.Sleep(25 * time.Millisecond)
		assert.Equal(t, 1, pd.numPreconnected())
		conn, err = pd.dialServer(nil, nil)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, fmt.Sprintf("preconnected-%d", i), conn.(*testConn).id, "Should have gotten preconnected connection")
	}

	for i := 1; i < maxSuccessfulDials; i++ {
		t.Log(i)
		time.Sleep(25 * time.Millisecond)
		conn, err = pd.dialServer(nil, nil)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, fmt.Sprintf("ondemand-%d", i+1), conn.(*testConn).id, "Should have gotten on demand dialed connection after preconnects started failing")
	}

	conn, err = pd.dialServer(nil, nil)
	assert.Error(t, err, "Should have failed to dial after on demand dials started failing")
}

func TestPreconnectingTimeout(t *testing.T) {
	expiration := 250 * time.Millisecond
	pd := newPreconnectingDialer("testPreconnectingTimeout", 2, expiration, newEmptyDialer())
	defer pd.close()

	for i := 1; i <= maxSuccessfulPreconnects; i++ {
		// dial to fill up pool
		pd.dialServer(nil, nil)
	}

	// wait for preconnected connections to expire
	time.Sleep(expiration * 2)
	conn, err := pd.dialServer(nil, nil)
	if assert.NoError(t, err) {
		assert.NotContains(t, conn.(*testConn).id, "preconnected", "After all preconnected connections expired, we should dial on demand")
	}
}

func TestPreconnectingClose(t *testing.T) {
	expiration := 250 * time.Millisecond
	pd := newPreconnectingDialer("testPreconnectingTimeout", 2, expiration, newEmptyDialer())

	for i := 1; i <= maxSuccessfulPreconnects; i++ {
		// dial to fill up pool
		pd.dialServer(nil, nil)
	}

	// wait a little for preconnecting to finish
	time.Sleep(25 * time.Millisecond)
	assert.True(t, len(pd.pool) >= 2, "should have at least 2 preconnections")
	assert.True(t, len(pd.pool) <= 4, "should have no more than 4 preconnections")

	pd.close()
	// wait a little for closing to happen
	time.Sleep(25 * time.Millisecond)
	assert.Empty(t, pd.pool)

	// wait for preconnected connections to expire
	time.Sleep(expiration * 2)
	conn, err := pd.dialServer(nil, nil)
	if assert.NoError(t, err) {
		assert.NotContains(t, conn.(*testConn).id, "preconnected", "After all preconnected connections expired, we should dial on demand")
	}
}
