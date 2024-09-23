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

func TestSelectorDial(t *testing.T) {
	tests := []struct {
		name     string
		assertFn func(t *testing.T)
	}{
		{name: "dial success", assertFn: assertSelectorSuccess},
		{name: "dialer advance", assertFn: assertSelectorAdvance},
		{name: "fail to dial", assertFn: assertSelectorFailToDial},
		{name: "unsupported addr", assertFn: assertSelectorUnsupportedAddr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assertFn(t)
		})
	}
}

func assertSelectorSuccess(t *testing.T) {
	selector := NewSelector(
		[]Dialer{
			&mockDialer{name: "dialer-0"},
			&mockDialer{name: "dialer-1"},
		},
	)
	_, err := selector.Dial("tcp", selectorTestAddr)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), selector.active.Load(), "dialer 0 should still be active dialer")
}

func assertSelectorAdvance(t *testing.T) {
	selector := NewSelector(
		[]Dialer{
			&mockDialer{name: "failingDialer", forceFail: true},
			&mockDialer{name: "succeedingDialer"},
		},
	)
	_, err := selector.Dial("tcp", selectorTestAddr)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), selector.active.Load(), "selector should have advanced to dialer 1")
}

func assertSelectorFailToDial(t *testing.T) {
	selector := NewSelector(
		[]Dialer{
			&mockDialer{name: "failingDialer-0", forceFail: true},
			&mockDialer{name: "failingDialer-1", forceFail: true},
		},
	)
	_, err := selector.Dial("tcp", selectorTestAddr)
	assert.Error(t, err)
}

func assertSelectorUnsupportedAddr(t *testing.T) {
	// we need to test that Dial fails when no dialer supports the addr to avoid infinite loop
	selector := NewSelector(
		[]Dialer{
			&mockDialer{name: "Microsoft"},
			&mockDialer{name: "Microsoft still"},
		},
	)
	_, err := selector.Dial("", "working windows updates")
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
