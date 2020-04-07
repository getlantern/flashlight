package chained

import (
	"bytes"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/deterministic"
	"github.com/getlantern/flashlight/geolookup"
	tls "github.com/refraction-networking/utls"
)

// We look up the client's geolocation to determine which browser to emulate.
const geoLookupTimeout = 2 * time.Second

type browserChoice struct {
	sessionTicketLifetime time.Duration
}

type weightedBrowserChoice struct {
	browserChoice
	marketShare float64
}

// Implements the deterministic.WeightedChoice interface.
func (rbd weightedBrowserChoice) Weight() int {
	return int(rbd.marketShare * 100)
}

var (
	// We treat Internet Explorer and Edge as the same thing below.

	// https://github.com/getlantern/lantern-internal/issues/3315#issue-560602994
	chrome     = browserChoice{30 * time.Minute}
	safari     = browserChoice{24 * time.Hour}
	firefox    = browserChoice{24 * time.Hour}
	edge       = browserChoice{10 * time.Hour}
	threeSixty = browserChoice{9 * time.Hour}
	qq         = browserChoice{30 * time.Minute}

	// https://gs.statcounter.com/browser-market-share#monthly-201910-201910-bar
	globalBrowserChoices = []deterministic.WeightedChoice{
		weightedBrowserChoice{chrome, 0.65},
		weightedBrowserChoice{safari, 0.17},
		weightedBrowserChoice{firefox, 0.04},
		weightedBrowserChoice{edge, 0.04},
	}

	// https://github.com/getlantern/lantern-internal/issues/3315#issuecomment-589253390
	browserChoicesByCountry = map[string][]deterministic.WeightedChoice{
		"CN": []deterministic.WeightedChoice{
			weightedBrowserChoice{edge, 0.36},
			weightedBrowserChoice{threeSixty, 0.26},
			weightedBrowserChoice{qq, 0.10},
			weightedBrowserChoice{firefox, 0.03},
		},
	}
)

// Chooses a TTL for session tickets for this client. This decision is made using data on market
// share for the top 4 browsers and the session ticket lifetimes enforced by those browsers.
//
// If necessary, we use region-specific market share figures. This is based on the client's
// geolocation and thus this function may block for a period while geolocation is determined.
func chooseSessionTicketTTL(uc common.UserConfig) time.Duration {
	countryCode := geolookup.GetCountry(geoLookupTimeout)
	if countryCode == "" {
		log.Error("failed to retrieve country code; using default session ticket lifetime settings")
	}
	choices, ok := browserChoicesByCountry[countryCode]
	if !ok {
		choices = globalBrowserChoices
	}
	userID, _ := strconv.ParseInt(uc.GetUserID(), 10, 64)
	choice := deterministic.MakeWeightedChoice(userID, choices)
	return choice.(weightedBrowserChoice).sessionTicketLifetime
}

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
