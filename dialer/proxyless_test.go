package dialer

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/stretchr/testify/assert"
)

type mockStreamDialer struct {
	dialStreamFunc func(ctx context.Context, addr string) (transport.StreamConn, error)
}

func (m *mockStreamDialer) DialStream(ctx context.Context, addr string) (transport.StreamConn, error) {
	return m.dialStreamFunc(ctx, addr)
}

func TestGetOrCreateDialer_ExistingDialer(t *testing.T) {
	// Arrange
	host := "example.com"
	addr := "example.com:443"
	mockDialer := &mockStreamDialer{
		dialStreamFunc: func(ctx context.Context, addr string) (transport.StreamConn, error) {
			return nil, nil
		},
	}
	successfulDialers.Store(host, mockDialer)
	defer successfulDialers.Delete(host)

	d := &proxylessDialer{}

	// Act
	dialer, err := d.getOrCreateDialer(context.Background(), host, addr)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, mockDialer, dialer)
}

func TestGetOrCreateDialer_NewDialerSuccess(t *testing.T) {
	// Arrange
	host := "example.com"
	addr := "example.com:443"
	mockDialer := &mockStreamDialer{}
	d := &proxylessDialer{
		newDialer: func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error) {
			return mockDialer, nil
		},
		configBytes: []byte("mock config"),
	}

	// Act
	dialer, err := d.getOrCreateDialer(context.Background(), host, addr)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, mockDialer, dialer)
}

func TestGetOrCreateDialer_NewDialerFailure(t *testing.T) {
	// Arrange
	host := "example.com"
	addr := "example.com:443"
	expectedErr := errors.New("failed to create dialer")
	d := &proxylessDialer{
		newDialer: func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error) {
			return nil, expectedErr
		},
		configBytes: []byte("mock config"),
	}

	// Act
	dialer, err := d.getOrCreateDialer(context.Background(), host, addr)

	// Assert
	assert.Nil(t, dialer)
	assert.EqualError(t, err, expectedErr.Error())
}
func TestDialContext_Success(t *testing.T) {
	successfulDialers = sync.Map{}
	failed = sync.Map{}
	// Arrange
	addr := "example.com:443"
	mockConn := &net.TCPConn{}
	mockDialer := &mockStreamDialer{}
	mockDialer.dialStreamFunc = func(ctx context.Context, addr string) (transport.StreamConn, error) {
		return mockConn, nil
	}
	d := &proxylessDialer{
		newDialer: func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error) {
			return mockDialer, nil
		},
		configBytes: []byte("mock config"),
	}

	// Act
	conn, err := d.DialContext(context.Background(), "tcp", addr)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, mockConn, conn)
}

func TestDialContext_CreateDialerError(t *testing.T) {
	successfulDialers = sync.Map{}
	failed = sync.Map{}
	// Arrange
	addr := "example.com:443"
	expectedErr := errors.New("failed to create dialer")
	d := &proxylessDialer{
		newDialer: func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error) {
			return nil, expectedErr
		},
		configBytes: []byte("mock config"),
	}

	// Act
	conn, err := d.DialContext(context.Background(), "tcp", addr)

	// Assert
	assert.Nil(t, conn)
	assert.EqualError(t, err, "failed to create smart dialer: failed to create dialer")
}

func TestDialContext_DialStreamError(t *testing.T) {
	successfulDialers = sync.Map{}
	failed = sync.Map{}
	// Arrange
	addr := "example.com:443"
	expectedErr := errors.New("failed to dial stream")
	mockDialer := &mockStreamDialer{}
	mockDialer.dialStreamFunc = func(ctx context.Context, addr string) (transport.StreamConn, error) {
		return nil, expectedErr
	}
	d := &proxylessDialer{
		newDialer: func(ctx context.Context, testDomains []string, configBytes []byte) (transport.StreamDialer, error) {
			return mockDialer, nil
		},
		configBytes: []byte("mock config"),
	}

	// Act
	conn, err := d.DialContext(context.Background(), "tcp", addr)

	// Assert
	assert.Nil(t, conn)
	assert.EqualError(t, err, "failed to dial stream with new dialer: failed to dial stream")
}
