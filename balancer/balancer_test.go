package balancer

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sort"
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
	recheckInterval = time.Millisecond
	evalInterval = 10 * time.Millisecond
}

func TestNoDialers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	b := newBalancer()
	_, err := b.DialContext(ctx, "tcp", addr)
	assert.Error(t, err, "Dialing with no dialers should have failed")
}

func TestSingleDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer := &testDialer{
		name:        "dialer1",
		rtt:         50 * time.Millisecond,
		bandwidth:   10000,
		successRate: 1,
	}
	// Test successful single dialer
	b := newBalancer(dialer)
	conn, err := b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing should have succeeded") {
		doTestConn(t, conn)
	}

	dialers := b.copyOfDialers()
	if assert.Len(t, dialers, 1) {
		assert.EqualValues(t, 1, dialers[0].Attempts())
		assert.EqualValues(t, 1, dialers[0].Successes())
		assert.EqualValues(t, 0, dialers[0].Failures())
	}

	// Test close balancer
	b.Close()
	time.Sleep(250 * time.Millisecond)
	assert.EqualValues(t, 1, atomic.LoadInt32(&dialer.stopped))
	_, err = b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing on closed balancer should fail") {
		assert.Contains(t, "No dialers", err.Error(), "Error should have mentioned that there were no dialers")
	}
}

func TestGoodSlowDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer1 := &testDialer{
		name:      "dialer1",
		rtt:       50 * time.Millisecond,
		bandwidth: 10000,
	}
	dialer2 := &testDialer{
		name:        "dialer2",
		rtt:         500 * time.Millisecond,
		bandwidth:   10000,
		successRate: 1,
	}

	b := newBalancer(dialer1)
	_, err := b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing bad dialer should fail")
	b.Reset([]Dialer{dialer1, dialer2})
	_, err = b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing with one good dialer should succeed") {
		assert.True(t, dialer1.Attempts() > 0, "should have tried fast dialer first")
	}
}

func TestAllFailingUpstream(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer1 := &testDialer{
		name:            "dialer1",
		rtt:             50 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: true,
		successRate:     1,
	}
	dialer2 := &testDialer{
		name:            "dialer2",
		rtt:             500 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: true,
		successRate:     1,
	}

	b := newBalancer(dialer1, dialer2)
	_, err := b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing all bad dialers should fail")
	assert.EqualValues(t, 0, dialer1.Failures(), "When all dialers fail upstream, don't record a failure")
	assert.EqualValues(t, 0, dialer2.Failures(), "When all dialers fail upstream, don't record a failure")
	assert.EqualValues(t, 1, dialer1.Attempts(), "All dialers should have had 1 attempt")
	assert.EqualValues(t, 1, dialer2.Attempts(), "All dialers should have had 1 attempt")
}

func TestOneFailingUpstream(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer1 := &testDialer{
		name:            "dialer1",
		rtt:             50 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: true,
		successRate:     1,
	}
	dialer2 := &testDialer{
		name:              "dialer2",
		rtt:               500 * time.Millisecond,
		bandwidth:         10000,
		successRate:       1,
		remainingFailures: 1,
	}

	b := newBalancer(dialer1, dialer2)
	_, err := b.Dial("tcp", addr)
	assert.NoError(t, err, "Dialing with one good dialer should succeed")
	assert.EqualValues(t, 1, dialer1.Failures(), "When a dialer succeeds, dialer that failed upstream should be marked as failed")
	assert.EqualValues(t, 1, dialer2.Failures(), "Dialer that failed on first dial should be marked as failed")
	assert.EqualValues(t, 1, dialer1.Attempts(), "Dialer failing upstream should have had only 1 attempt")
	assert.EqualValues(t, 2, dialer2.Attempts(), "Dialer that failed on first dial should have had 2 attempts")
}

func TestTrusted(t *testing.T) {
	dialer := &testDialer{
		untrusted:   1,
		successRate: 1,
	}

	_, err := newBalancer(dialer).Dial("", "does-not-exist.com:80")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.EqualValues(t, 0, dialer.Attempts(), "should not dial untrusted dialer")

	_, err = newBalancer(dialer).Dial("", "does-not-exist.com:8080")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.EqualValues(t, 0, dialer.Attempts(), "should not dial untrusted dialer")

	atomic.StoreInt32(&dialer.untrusted, 0)
	_, err = newBalancer(dialer).Dial("", "does-not-exist.com:80")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.EqualValues(t, 1, dialer.Attempts(), "should dial trusted dialer")
	_, err = newBalancer(dialer).Dial("", "does-not-exist.com:8080")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.EqualValues(t, 2, dialer.Attempts(), "should dial trusted dialer")
}

func TestSwitchAwayFromSlowDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	slowDialer := &testDialer{
		name:        "slow",
		rtt:         100 * time.Millisecond,
		bandwidth:   10000,
		successRate: 1,
	}
	fastDialer := &testDialer{
		name:        "fast",
		rtt:         10 * time.Millisecond,
		bandwidth:   10000,
		successRate: 1,
	}

	b := newBalancer(slowDialer, fastDialer)
	_, err := b.Dial("tcp", addr)
	assert.NoError(t, err, "Dialing with good dialers should succeed")
	// simulate generic network issue
	slowDialer.setSuccessRate(0.1)
	fastDialer.setSuccessRate(0.1)
	_, err = b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing should fail when no succeeding dailers")
	slowDialer.setSuccessRate(1)
	_, err = b.Dial("tcp", addr)
	assert.NoError(t, err, "Dialing should succeed with one good dialer")
	assert.Equal(t, slowDialer, b.copyOfDialers()[0], "should switch to slow dialer immediately if fast one fails")
	fastDialer.setSuccessRate(1)
	// wait a bit longer for probing to complete
	time.Sleep(time.Second)
	_, err = b.Dial("tcp", addr)
	assert.NoError(t, err, "Dialing should succeed with good dialers")
	assert.Equal(t, fastDialer, b.copyOfDialers()[0], "The fast dialer should rise to the top")
}

func TestSorting(t *testing.T) {
	dialers := sortedDialers{
		// Order known dialers by bandwidth / RTT
		&testDialer{
			name:        "1",
			bandwidth:   1000,
			rtt:         1 * time.Millisecond,
			successRate: 1,
		},
		&testDialer{
			name:        "2",
			bandwidth:   10000,
			rtt:         15 * time.Millisecond,
			successRate: 1,
		},
		// Unknown bandwidth should be avoided
		&testDialer{
			name:        "3",
			bandwidth:   0,
			successRate: 1,
		},
		// Within unknown bandwidth, sort by name
		&testDialer{
			name:        "4",
			bandwidth:   0,
			successRate: 1,
		},
		// Same ordering as above applies to failing proxies, which all come after
		// succeeding ones
		&testDialer{
			name:      "5",
			bandwidth: 1000,
			rtt:       1 * time.Millisecond,
		},
		&testDialer{
			name:      "6",
			bandwidth: 10000,
			rtt:       15 * time.Millisecond,
		},
		&testDialer{
			name:      "7",
			bandwidth: 0,
		},
		&testDialer{
			name:      "8",
			bandwidth: 0,
		},
	}

	// Shuffle and sort multiple times to make sure that comparisons work in both
	// directions
	failingSortedRandomlyAtLeastOnce := false
	for i := 0; i < 500; i++ {
		rand.Shuffle(len(dialers), func(i, j int) {
			dialers[i], dialers[j] = dialers[j], dialers[i]
		})
		sort.Sort(dialers)
		var order []string
		for _, d := range dialers {
			order = append(order, d.Name())
		}

		assert.EqualValues(t, []string{"1", "2", "3", "4"}, order[:4], "Succeeding dialers should sort predictably")
		for i := 4; i < len(order); i++ {
			if fmt.Sprintf("%d", i+1) != order[i] {
				failingSortedRandomlyAtLeastOnce = true
			}
		}
	}

	assert.True(t, failingSortedRandomlyAtLeastOnce)
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

func newBalancer(dialers ...Dialer) *Balancer {
	return New(1*time.Second, dialers...)
}
