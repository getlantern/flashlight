package balancer

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	msg = []byte("Hello world")
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestNoDialers(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	b := newBalancer(Sticky)
	_, err := b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing with no dialers should have failed")
}

func TestSingleDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer := newDialer(1)
	dialerClosed := int32(0)
	dialer.OnClose = func() {
		atomic.StoreInt32(&dialerClosed, 1)
	}
	// Test successful single dialer
	b := newBalancer(Sticky, dialer)
	conn, err := b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing should have succeeded") {
		doTestConn(t, conn)
	}

	if assert.Len(t, b.dialers.dialers, 1) {
		assert.EqualValues(t, 1, b.dialers.dialers[0].stats.attempts)
		assert.EqualValues(t, 1, b.dialers.dialers[0].stats.successes)
		assert.EqualValues(t, 0, b.dialers.dialers[0].stats.failures)
	}

	// Test close balancer
	b.Close()
	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&dialerClosed), "Dialer should have been closed")
	_, err = b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing on closed balancer should fail") {
		assert.Contains(t, "No dialers", err.Error(), "Error should have mentioned that there were no dialers")
	}
}

func TestRetryOnBadDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	d1Attempts := int32(0)
	dialer1 := newCondDialer(1, func() bool { atomic.AddInt32(&d1Attempts, 1); return true })
	d2Attempts := int32(0)
	dialer2 := newCondDialer(2, func() bool { atomic.AddInt32(&d2Attempts, 1); return true })

	b := newBalancer(Sticky, dialer1)
	_, err := b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing bad dialer should fail") {
		assert.EqualValues(t, 1, atomic.LoadInt32(&d1Attempts), "should try same dialer only once")
	}
	b.Reset(dialer1, dialer2)
	_, err = b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing bad dialer should fail") {
		assert.EqualValues(t, dialAttempts, atomic.LoadInt32(&d1Attempts)+atomic.LoadInt32(&d2Attempts), "should try enough times when there are more then 1 dialer")
	}
}

func TestRandomDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	d1Attempts := int32(0)
	// sleep for a while to avoid dial time from varying too much, same below
	dialer1 := newCondDialer(1, func() bool {
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&d1Attempts, 1)
		return false
	})
	d2Attempts := int32(0)
	dialer2 := newCondDialer(2, func() bool {
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&d2Attempts, 1)
		return false
	})
	d3Attempts := int32(0)
	dialer3 := newCondDialer(3, func() bool {
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&d3Attempts, 1)
		return false
	})

	b := newBalancer(Random, dialer1, dialer2, dialer3)
	defer b.Close()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_, err := b.Dial("tcp", addr)
				assert.NoError(t, err, "Dialing should have succeeded")
			}
		}()
	}
	wg.Wait()
	assertWithinRangeOf(t, atomic.LoadInt32(&d1Attempts), 1000, 200)
	assertWithinRangeOf(t, atomic.LoadInt32(&d2Attempts), 1000, 200)
	assertWithinRangeOf(t, atomic.LoadInt32(&d3Attempts), 1000, 200)
}

func assertWithinRangeOf(t *testing.T, actual int32, expected int32, margin int32) {
	assert.True(t, actual >= expected-margin && actual <= expected+margin, fmt.Sprintf("%v not within %v of %v", actual, margin, expected))
}

func TestSuccessWithCondDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	dialer1 := newCondDialer(1, func() bool { return true })
	dialer2 := newDialer(2)
	dialer3 := newDialer(3)

	// Test success with failing dialer
	b := newBalancer(Sticky, dialer1, dialer2, dialer3)
	defer b.Close()
	conn, err := b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing should have succeeded") {
		doTestConn(t, conn)
	}
}

func TestRecheck(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	attempts := int32(0)
	dialer := newCondDialer(1, func() bool { return atomic.AddInt32(&attempts, 1) <= 1 })
	// Test failure
	b := newBalancer(Sticky, dialer, dialer)
	_, err := b.Dial("tcp", addr)
	assert.NoError(t, err, "Dialing should have succeeded as we have 2nd try")
	assert.EqualValues(t, 2, atomic.LoadInt32(&attempts), "Wrong number of dial attempts on failed dialer")

	// Test success after successful retest using default check
	conn, err := b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing should have succeeded") {
		doTestConn(t, conn)
	}
}

func TestTrusted(t *testing.T) {
	dialCount := 0
	dialer := &Dialer{
		DialFN: func(network, addr string) (net.Conn, error) {
			dialCount++
			return nil, nil
		},
	}

	_, err := newBalancer(Sticky, dialer).Dial("tcp", "does-not-exist.com:80")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.Equal(t, dialCount, 0, "should not dial untrusted dialer")

	_, err = newBalancer(Sticky, dialer).Dial("tcp", "does-not-exist.com:8080")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.Equal(t, dialCount, 0, "should not dial untrusted dialer")

	dialer.Trusted = true
	_, err = newBalancer(Sticky, dialer).Dial("tcp", "does-not-exist.com:80")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.Equal(t, dialCount, 1, "should dial untrusted dialer")
	_, err = newBalancer(Sticky, dialer).Dial("tcp", "does-not-exist.com:8080")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.Equal(t, dialCount, 2, "should dial untrusted dialer")
}

func TestCheck(t *testing.T) {
	minCheckInterval := 50 * time.Millisecond
	var checkCount uint32
	d := &Dialer{
		DialFN: func(network, addr string) (net.Conn, error) {
			return nil, nil
		},
		Check: func(checkData interface{}, onFailure func(string)) (bool, time.Duration) {
			newCount := atomic.AddUint32(&checkCount, 1)
			log.Debugf("Check() called %d times", newCount)
			return true, 1 * time.Second
		},
		Trusted: true,
	}
	New(&Opts{
		Strategy:         Sticky,
		Dialers:          []*Dialer{d},
		MinCheckInterval: minCheckInterval,
		MaxCheckInterval: minCheckInterval,
	})

	time.Sleep(minCheckInterval * 2)
	assert.True(t, atomic.LoadUint32(&checkCount) >= 2)
}

func TestResetDailers(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	chReset := make(chan struct{})
	chContinue := make(chan struct{})
	bad := int32(0)
	badDialer := newCondDialer(1, func() bool {
		atomic.AddInt32(&bad, 1)
		chReset <- struct{}{}
		<-chContinue
		return true
	})
	good := int32(0)
	goodDialer := newCondDialer(2, func() bool {
		atomic.AddInt32(&good, 1)
		return false
	})
	b := newBalancer(Sticky, badDialer)
	go func() {
		<-chReset
		b.Reset(goodDialer)
		chContinue <- struct{}{}
	}()
	_, err := b.Dial("tcp", addr)
	assert.NoError(t, err, "Should have no error dialing with resetted balancer")
	assert.Equal(t, int32(1), atomic.LoadInt32(&bad), "should dial bad dialer only once")
	assert.Equal(t, int32(1), atomic.LoadInt32(&good), "should dial good dialer only once")
}

func TestOnRequest(t *testing.T) {
	reqs := make(chan bool, 1)
	dialer := &Dialer{
		Label: "OnRequestTestDialer",
		DialFN: func(network, addr string) (net.Conn, error) {
			return net.Dial(network, addr)
		},
		OnRequest: func(req *http.Request) {
			reqs <- true
		},
	}

	b := newBalancer(Sticky, dialer)
	defer b.Close()

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	b.OnRequest(req)

	received := 0
	select {
	case <-reqs:
		received++
		break
	case <-time.After(time.Second * 4):
		break
	}

	assert.Equal(t, 1, received)
}

func newDialer(id int) *Dialer {
	dialer := &Dialer{
		Label: fmt.Sprintf("Dialer %d", id),
		DialFN: func(network, addr string) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	return dialer
}

func newLatencyDialer(id int, latency time.Duration, delta time.Duration, attempts *int32) *Dialer {
	dialer := &Dialer{
		Label: fmt.Sprintf("Dialer %d", id),
		DialFN: func(network, addr string) (net.Conn, error) {
			t := int64(latency) + rand.Int63n(int64(delta)*2) - int64(delta)
			time.Sleep(time.Duration(t))
			atomic.AddInt32(attempts, 1)
			return net.Dial(network, addr)
		},
	}
	return dialer
}

// newCondDialer creates a dialer that will fail if beforeDial returns true.
func newCondDialer(id int32, beforeDial func() bool) *Dialer {
	d := &Dialer{
		Label: "Dialer " + strconv.Itoa(int(id)),
		DialFN: func(network, addr string) (net.Conn, error) {
			if beforeDial() {
				return nil, fmt.Errorf("Failing intentionally")
			}
			return net.Dial(network, addr)
		},
	}
	return d
}

func doTestConn(t *testing.T, conn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		n, err := conn.Write(msg)
		assert.NoError(t, err, "Writing should have succeeded")
		assert.Equal(t, len(msg), n, "Should have written full message")
		wg.Done()
	}()
	go func() {
		b := make([]byte, len(msg))
		n, err := io.ReadFull(conn, b)
		assert.NoError(t, err, "Read should have succeeded")
		assert.Equal(t, len(msg), n, "Should have read full message")
		assert.Equal(t, msg, b[:n], "Read should have matched written")
		wg.Done()
	}()

	wg.Wait()
	err := conn.Close()
	assert.NoError(t, err, "Should close conn")
}

func newBalancer(strategy Strategy, dialers ...*Dialer) *Balancer {
	return New(&Opts{Strategy: strategy, Dialers: dialers})
}
