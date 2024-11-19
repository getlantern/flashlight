// Description: Tests for the connectivity check dialer.
package dialer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOnConnected(t *testing.T) {
	mockDialer1 := new(mockProxyDialer)
	mockDialer2 := new(mockProxyDialer)
	mockDialer3 := new(mockProxyDialer)

	opts := &Options{
		OnError:   func(err error, hasSucceeding bool) {},
		OnSuccess: func(pd ProxyDialer) {},
	}

	fcd := newFastConnectDialer(opts, nil)

	// Test adding the first dialer
	fcd.onConnected(mockDialer1, 100*time.Millisecond)
	assert.Equal(t, 1, len(fcd.connected.dialers))
	assert.Equal(t, mockDialer1, fcd.topDialer.get())

	// Test adding a faster dialer
	fcd.onConnected(mockDialer2, 50*time.Millisecond)
	assert.Equal(t, 2, len(fcd.connected.dialers))
	assert.Equal(t, mockDialer2, fcd.topDialer.get())

	// Test adding a slower dialer
	fcd.onConnected(mockDialer1, 150*time.Millisecond)
	assert.Equal(t, 3, len(fcd.connected.dialers))
	assert.Equal(t, mockDialer2, fcd.topDialer.get())

	// Test adding a new fastest dialer
	fcd.onConnected(mockDialer3, 10*time.Millisecond)
	assert.Equal(t, 4, len(fcd.connected.dialers))
	assert.Equal(t, mockDialer3, fcd.topDialer.get())
}
func TestConnectAll(t *testing.T) {
	mockDialer1 := new(mockProxyDialer)
	mockDialer2 := new(mockProxyDialer)
	mockDialer3 := new(mockProxyDialer)

	opts := &Options{
		OnError:   func(err error, hasSucceeding bool) {},
		OnSuccess: func(pd ProxyDialer) {},
	}

	fcd := newFastConnectDialer(opts, func(opts *Options, existing Dialer) Dialer {
		return nil
	})

	dialers := []ProxyDialer{mockDialer1, mockDialer2, mockDialer3}

	// Test connecting with multiple dialers
	fcd.connectAll(dialers)

	// Sleep for a bit to allow the goroutines to finish while checking for
	// the connected dialers
	tries := 0
	for len(fcd.connected.dialers) < 3 && tries < 100 {
		time.Sleep(10 * time.Millisecond)
		tries++
	}
	assert.Equal(t, 3, len(fcd.connected.dialers))
	assert.NotNil(t, fcd.topDialer.get())

	// Test with no dialers
	fcd = newFastConnectDialer(opts, nil)
	fcd.connectAll([]ProxyDialer{})
	assert.Equal(t, 0, len(fcd.connected.dialers))
	assert.Nil(t, fcd.topDialer.get())
}
