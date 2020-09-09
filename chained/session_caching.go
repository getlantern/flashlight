package chained

import (
	"bytes"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"
)

// expiringSessionCache is a tls.ClientSessionCache that expires tickets older than the
// configured TTL. Because we use one of these per proxy server, it does not need to care about
// session key names nor LRU logic.
type expiringSessionCache struct {
	sync.RWMutex

	server       string
	ttl          time.Duration
	defaultState *tls.ClientSessionState
	currentState *tls.ClientSessionState
	lastUpdated  time.Time
}

// newExpiringSessionCache returns an expiringSessionCache. It initializes the current state from whatever is stored on disk
// for the given server. For this to work, PersistSessionStates has to be called prior to creating this cache.
func newExpiringSessionCache(server string, ttl time.Duration, defaultState *tls.ClientSessionState) *expiringSessionCache {
	currentState, lastUpdated := persistedSessionStateFor(server)
	if currentState != nil {
		log.Debugf("Found persisted session state for %v with timestamp %v", server, lastUpdated)
	}
	return &expiringSessionCache{
		server:       server,
		ttl:          ttl,
		defaultState: defaultState,
		currentState: currentState,
		lastUpdated:  lastUpdated,
	}
}

// Put adds the provided cs to the cache. It does nothing if cs is nil in
// thinking that the handshake failure when resuming session may be temporary.
// sessionKey is required by the ClientSessionCache interface but ignored here.
func (c *expiringSessionCache) Put(sessionKey string, cs *tls.ClientSessionState) {
	if cs == nil {
		return
	}
	c.Lock()
	defer c.Unlock()

	if c.currentState != nil && bytes.Equal(c.currentState.SessionTicket(), cs.SessionTicket()) {
		// same as the old ticket, don't bother updating and leave timestamp alone
		return
	}

	c.currentState = cs
	c.lastUpdated = time.Now()
	saveSessionState(c.server, c.currentState, c.lastUpdated)
}

// Get returns the cached ClientSessionState if it's not expired, or
// defaultState. sessionKey is required by the ClientSessionCache interface but
// ignored here.
func (c *expiringSessionCache) Get(sessionKey string) (*tls.ClientSessionState, bool) {
	c.RLock()
	defer c.RUnlock()

	state := c.currentState
	if state == nil || time.Since(c.lastUpdated) > c.ttl {
		state = c.defaultState
	}

	return state, state != nil
}
