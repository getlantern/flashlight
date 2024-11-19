package dialer

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClone(t *testing.T) {
	// Create a sample Options object
	original := &Options{
		Dialers: []ProxyDialer{
			&mockProxyDialer{name: "dialer1"},
			&mockProxyDialer{name: "dialer2"},
		},
		OnError: func(err error, retry bool) {
			// Sample error handler
		},
		OnSuccess: func(dialer ProxyDialer) {
			// Sample success handler
		},
	}

	// Clone the original Options object
	cloned := original.Clone()

	// Verify that the cloned object is not nil
	assert.NotNil(t, cloned)

	// Verify that the cloned object is not the same as the original
	assert.NotSame(t, original, cloned)

	// Verify that the fields are correctly cloned
	assert.Equal(t, original.Dialers, cloned.Dialers)
}

// mockProxyDialer is a mock implementation of the ProxyDialer interface for testing purposes
type mockProxyDialer struct {
	name string
}

func (m *mockProxyDialer) DialProxy(ctx context.Context) (net.Conn, error) {
	return nil, nil
}

func (m *mockProxyDialer) SupportsAddr(network, addr string) bool {
	return true
}

func (m *mockProxyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, bool, error) {
	return nil, false, nil
}

func (m *mockProxyDialer) Name() string {
	return m.name
}

func (m *mockProxyDialer) Label() string {
	return m.name
}

func (m *mockProxyDialer) JustifiedLabel() string {
	return m.name
}

func (m *mockProxyDialer) Location() (string, string, string) {
	return "", "", ""
}

func (m *mockProxyDialer) Protocol() string {
	return "mock"
}

func (m *mockProxyDialer) Addr() string {
	return "mock"
}

func (m *mockProxyDialer) Trusted() bool {
	return true
}

func (m *mockProxyDialer) NumPreconnecting() int {
	return 0
}

func (m *mockProxyDialer) NumPreconnected() int {
	return 0
}

func (m *mockProxyDialer) MarkFailure() {}

func (m *mockProxyDialer) EstRTT() time.Duration {
	return 0
}

func (m *mockProxyDialer) EstBandwidth() float64 {
	return 0
}

func (m *mockProxyDialer) EstSuccessRate() float64 {
	return 0
}

func (m *mockProxyDialer) Attempts() int64 {
	return 0
}

func (m *mockProxyDialer) Successes() int64 {
	return 0
}

func (m *mockProxyDialer) ConsecSuccesses() int64 {
	return 0
}

func (m *mockProxyDialer) Failures() int64 {
	return 0
}

func (m *mockProxyDialer) ConsecFailures() int64 {
	return 0
}

func (m *mockProxyDialer) Succeeding() bool {
	return true
}

func (m *mockProxyDialer) DataSent() uint64 {
	return 0
}

func (m *mockProxyDialer) DataRecv() uint64 {
	return 0
}

func (m *mockProxyDialer) Ready() <-chan error {
	return nil
}

func (m *mockProxyDialer) Stop() {}

func (m *mockProxyDialer) WriteStats(w io.Writer) {}
