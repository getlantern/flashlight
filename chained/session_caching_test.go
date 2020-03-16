package chained

import (
	"testing"
	"time"

	tls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/assert"
)

func TestSessionCache(t *testing.T) {
	ttl := 10 * time.Millisecond
	defaultState := tls.MakeClientSessionState([]uint8{1, 2, 3}, 0, 0, nil, nil, nil)
	cache := newExpiringSessionCache("server", ttl, defaultState)
	someState := tls.MakeClientSessionState([]uint8{2, 3, 4}, 0, 0, nil, nil, nil)

	state, got := cache.Get("key")
	assert.True(t, got)
	assert.Equal(t, state, defaultState, "Should get the default state if nothing is in the cache")

	cache.Put("key", someState)
	state, got = cache.Get("key")
	assert.True(t, got)
	assert.Equal(t, state, someState, "Should get the state if it's in the cache")

	cache.Put("key", nil)
	state, got = cache.Get("key")
	assert.True(t, got)
	assert.Equal(t, state, someState, "Should skip putting an empty session state to the cache")

	time.Sleep(ttl)
	state, got = cache.Get("key")
	assert.True(t, got)
	assert.Equal(t, state, defaultState, "Should get the default state after the state expires")
}
