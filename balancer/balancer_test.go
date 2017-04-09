package balancer

import (
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
}

type testDialer struct {
	name      string
	latency   time.Duration
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
		conn, err = net.DialTimeout(network, addr, 250*time.Millisecond)
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

func TestNoDialers(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()
	b := New()
	_, err := b.Dial("tcp", addr)
	assert.Error(t, err, "Dialing with no dialers should have failed")
}

func TestSingleDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer := &testDialer{
		name:      "dialer1",
		latency:   50 * time.Millisecond,
		bandwidth: 10000,
	}
	// Test successful single dialer
	b := New(dialer)
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

func TestRetryOnBadDialer(t *testing.T) {
	addr, l := echoServer()
	defer func() { _ = l.Close() }()

	dialer1 := &testDialer{
		name:      "dialer1",
		latency:   50 * time.Millisecond,
		bandwidth: 10000,
		failing:   true,
	}
	dialer2 := &testDialer{
		name:      "dialer1",
		latency:   50 * time.Millisecond,
		bandwidth: 10000,
		failing:   true,
	}

	b := New(dialer1)
	_, err := b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing bad dialer should fail") {
		assert.EqualValues(t, 1, dialer1.Attempts(), "should try same dialer only once")
	}
	b.Reset(dialer1, dialer2)
	_, err = b.Dial("tcp", addr)
	if assert.Error(t, err, "Dialing bad dialer should fail") {
		assert.EqualValues(t, 3, dialer1.Attempts()+dialer2.Attempts(), "should try both dialers again")
	}
}

func TestTrusted(t *testing.T) {
	dialer := &testDialer{
		untrusted: true,
	}

	_, err := New(dialer).Dial("", "does-not-exist.com:80")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.EqualValues(t, 0, dialer.Attempts(), "should not dial untrusted dialer")

	_, err = New(dialer).Dial("", "does-not-exist.com:8080")
	assert.Error(t, err, "Dialing with no trusted dialers should have failed")
	assert.EqualValues(t, 0, dialer.Attempts(), "should not dial untrusted dialer")

	dialer.untrusted = false
	_, err = New(dialer).Dial("", "does-not-exist.com:80")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.EqualValues(t, 1, dialer.Attempts(), "should dial trusted dialer")
	_, err = New(dialer).Dial("", "does-not-exist.com:8080")
	assert.NoError(t, err, "Dialing with trusted dialer should have succeeded")
	assert.EqualValues(t, 2, dialer.Attempts(), "should dial trusted dialer")
}

func TestSorting(t *testing.T) {
	dialers := sortedDialers{
		// Unknown bandwidth comes first
		&testDialer{
			name:      "1",
			bandwidth: 0,
		},
		// Within unknown bandwidth, sort by name
		&testDialer{
			name:      "2",
			bandwidth: 0,
		},
		// Order known dialers by bandwidth / latency
		&testDialer{
			name:      "3",
			bandwidth: 1000,
			latency:   1 * time.Millisecond,
		},
		&testDialer{
			name:      "4",
			bandwidth: 10000,
			latency:   15 * time.Millisecond,
		},
		// Same ordering as above applies to failing proxies, which all come after
		// succeeding ones
		&testDialer{
			name:      "5",
			bandwidth: 0,
			failing:   true,
		},
		&testDialer{
			name:      "6",
			bandwidth: 0,
			failing:   true,
		},
		&testDialer{
			name:      "7",
			bandwidth: 1000,
			latency:   1 * time.Millisecond,
			failing:   true,
		},
		&testDialer{
			name:      "8",
			bandwidth: 10000,
			latency:   15 * time.Millisecond,
			failing:   true,
		},
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
