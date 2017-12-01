package balancer

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"sort"
	"sync"
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
	b := newBalancer()
	_, err := b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing with no dialers should have failed")
}

func TestSingleDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer := start(&testDialer{
		name:      "dialer1",
		latency:   50 * time.Millisecond,
		bandwidth: 10000,
	})
	// Test successful single dialer
	b := newBalancer(dialer)
	conn, err := b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing should have succeeded") {
		doTestConn(t, conn)
	}

	if assert.Len(t, b.dialers, 1) {
		assert.EqualValues(t, 1, b.dialers[0].Attempts())
		assert.EqualValues(t, 1, b.dialers[0].Successes())
		assert.EqualValues(t, 0, b.dialers[0].Failures())
	}

	// Test close balancer
	b.Close()
	time.Sleep(250 * time.Millisecond)
	assert.True(t, dialer.stopped)
	_, err = b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing on closed balancer should fail") {
		assert.Contains(t, "No dialers", err.Error(), "Error should have mentioned that there were no dialers")
	}
}

func TestGoodSlowDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer1 := start(&testDialer{
		name:      "dialer1",
		latency:   50 * time.Millisecond,
		bandwidth: 10000,
		failing:   true,
	})
	dialer2 := start(&testDialer{
		name:      "dialer1",
		latency:   500 * time.Millisecond,
		bandwidth: 10000,
		failing:   false,
	})

	b := newBalancer(dialer1)
	_, err := b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing bad dialer should fail")
	b.Reset(dialer1, dialer2)
	_, err = b.Dial("tcp", addr)
	if assert.NoError(t, err, "Dialing with one good dialer should succeed") {
		assert.True(t, dialer1.Attempts() > 0, "should have tried fast dialer first")
	}
}

func TestAllFailingUpstream(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer1 := start(&testDialer{
		name:            "dialer1",
		latency:         50 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: true,
	})
	dialer2 := start(&testDialer{
		name:            "dialer2",
		latency:         500 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: true,
	})

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

	dialer1 := start(&testDialer{
		name:            "dialer1",
		latency:         50 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: true,
	})
	dialer2 := start(&testDialer{
		name:            "dialer2",
		latency:         500 * time.Millisecond,
		bandwidth:       10000,
		failingUpstream: false,
	})

	b := newBalancer(dialer1, dialer2)
	_, err := b.Dial("tcp", addr)
	assert.NoError(t, err, "Dialing with one good dialer should succeed")
	assert.EqualValues(t, 1, dialer1.Failures(), "When a dialer succeeds, dialer that failed upstream should be marked as failed")
	assert.EqualValues(t, 1, dialer1.Attempts(), "Dialer failing upstream should have had only 1 attempt")
	assert.EqualValues(t, 0, dialer2.Failures(), "Succeeding dialer should not be marked as failed")
}

func TestTrusted(t *testing.T) {
	dialer := start(&testDialer{
		untrusted: true,
	})

	_, err := newBalancer(dialer).Dial("", "does-not-exist.com:80")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.EqualValues(t, 0, dialer.Attempts(), "should not dial untrusted dialer")

	_, err = newBalancer(dialer).Dial("", "does-not-exist.com:8080")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.EqualValues(t, 0, dialer.Attempts(), "should not dial untrusted dialer")

	dialer.untrusted = false
	_, err = newBalancer(dialer).Dial("", "does-not-exist.com:80")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.EqualValues(t, 1, dialer.Attempts(), "should dial trusted dialer")
	_, err = newBalancer(dialer).Dial("", "does-not-exist.com:8080")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.EqualValues(t, 2, dialer.Attempts(), "should dial trusted dialer")
}

func TestSorting(t *testing.T) {
	dialers := sortedDialers{
		// Unknown bandwidth comes first
		start(&testDialer{
			name:      "1",
			bandwidth: 0,
		}),
		// Within unknown bandwidth, sort by name
		start(&testDialer{
			name:      "2",
			bandwidth: 0,
		}),
		// Order known dialers by bandwidth / latency
		start(&testDialer{
			name:      "3",
			bandwidth: 1000,
			latency:   1 * time.Millisecond,
		}),
		start(&testDialer{
			name:      "4",
			bandwidth: 10000,
			latency:   15 * time.Millisecond,
		}),
		// Same ordering as above applies to failing proxies, which all come after
		// succeeding ones
		start(&testDialer{
			name:      "5",
			bandwidth: 0,
			failing:   true,
		}),
		start(&testDialer{
			name:      "6",
			bandwidth: 0,
			failing:   true,
		}),
		start(&testDialer{
			name:      "7",
			bandwidth: 1000,
			latency:   1 * time.Millisecond,
			failing:   true,
		}),
		start(&testDialer{
			name:      "8",
			bandwidth: 10000,
			latency:   15 * time.Millisecond,
			failing:   true,
		}),
	}

	// Shuffle and sort multiple times to make sure that comparisons work in both
	// directions
	failingSortedRandomlyAtLeastOnce := false
	for i := 0; i < 500; i++ {
		// shuffle
		for i := range dialers {
			j := rand.Intn(i + 1)
			dialers[i], dialers[j] = dialers[j], dialers[i]
		}

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
	return New(250*time.Millisecond, 1*time.Second, dialers...)
}
