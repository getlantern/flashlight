package chained

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoopDialerDial(t *testing.T) {
	tests := []struct {
		name     string
		assertFn func(t *testing.T)
	}{
		{name: "dial success", assertFn: assertLoopDialerSuccess},
		{name: "dialer advance", assertFn: assertLoopDialerAdvance},
		{name: "fail to dial", assertFn: assertLoopDialerFailToDial},
		{name: "unsupported addr", assertFn: assertLoopDialerUnsupportedAddr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assertFn(t)
		})
	}
}

func assertLoopDialerSuccess(t *testing.T) {
	ld := NewLoopDialer(
		[]Dialer{
			&mockDialer{name: "dialer-0"},
			&mockDialer{name: "dialer-1"},
		},
	)
	_, err := ld.Dial("tcp", selectorTestAddr)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ld.active.Load(), "dialer 0 should still be active dialer")
}

func assertLoopDialerAdvance(t *testing.T) {
	ld := NewLoopDialer(
		[]Dialer{
			&mockDialer{name: "failingDialer", forceFail: true},
			&mockDialer{name: "succeedingDialer"},
		},
	)
	_, err := ld.Dial("tcp", selectorTestAddr)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), ld.active.Load(), "loopDialer should have advanced to dialer 1")
}

func assertLoopDialerFailToDial(t *testing.T) {
	ld := NewLoopDialer(
		[]Dialer{
			&mockDialer{name: "failingDialer-0", forceFail: true},
			&mockDialer{name: "failingDialer-1", forceFail: true},
		},
	)
	_, err := ld.Dial("tcp", selectorTestAddr)
	assert.Error(t, err)
}

func assertLoopDialerUnsupportedAddr(t *testing.T) {
	// we need to test that Dial fails when no dialer supports the addr to avoid infinite loop
	ld := NewLoopDialer(
		[]Dialer{
			&mockDialer{name: "Microsoft"},
			&mockDialer{name: "Microsoft still"},
		},
	)
	_, err := ld.Dial("", "working windows updates")
	assert.Error(t, err)
}

const selectorTestAddr = "http://test.addr"

type mockDialer struct {
	name           string
	consecFailures int64
	forceFail      bool
}

func (mockdialer *mockDialer) DialContext(ctx context.Context, network string, addr string) (conn net.Conn, failedUpstream bool, err error) {
	if mockdialer.forceFail {
		return nil, false, errors.New("failed to dial")
	}

	if addr != selectorTestAddr {
		return nil, false, errors.New("unsupported addr")
	}

	return nil, false, nil
}

func (mockdialer *mockDialer) MarkFailure() {
	mockdialer.consecFailures++
}

func (mockdialer *mockDialer) SupportsAddr(network string, addr string) bool {
	return addr == selectorTestAddr
}

func (mockdialer *mockDialer) ConsecFailures() int64 { return mockdialer.consecFailures }

func (mockdialer *mockDialer) DialProxy(ctx context.Context) (net.Conn, error) {
	return nil, errors.New("not implemented")
}

func (mockdialer *mockDialer) Name() string                       { return mockdialer.name }
func (mockdialer *mockDialer) Label() string                      { return mockdialer.name }
func (mockdialer *mockDialer) JustifiedLabel() string             { return mockdialer.name }
func (mockdialer *mockDialer) Location() (string, string, string) { return "", "", "" }
func (mockdialer *mockDialer) Protocol() string                   { return "mockdialer" }
func (mockdialer *mockDialer) Addr() string                       { return "mockdialer" }

func (mockdialer *mockDialer) Attempts() int64         { return 0 }
func (mockdialer *mockDialer) Successes() int64        { return 0 }
func (mockdialer *mockDialer) Failures() int64         { return 0 }
func (mockdialer *mockDialer) Trusted() bool           { return true }
func (mockdialer *mockDialer) NumPreconnecting() int   { return 0 }
func (mockdialer *mockDialer) NumPreconnected() int    { return 0 }
func (mockdialer *mockDialer) EstRTT() time.Duration   { return time.Millisecond }
func (mockdialer *mockDialer) EstBandwidth() float64   { return 0 }
func (mockdialer *mockDialer) EstSuccessRate() float64 { return 0 }
func (mockdialer *mockDialer) ConsecSuccesses() int64  { return 0 }
func (mockdialer *mockDialer) Succeeding() bool        { return true }
func (mockdialer *mockDialer) DataSent() uint64        { return 0 }
func (mockdialer *mockDialer) DataRecv() uint64        { return 0 }
func (mockdialer *mockDialer) Stop()                   {}
func (mockdialer *mockDialer) WriteStats(w io.Writer)  {}
