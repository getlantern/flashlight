package chained

import (
	"bytes"
	"container/list"
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
	choice := deterministic.MakeWeightedChoice(uc.GetUserID(), choices)
	return choice.(weightedBrowserChoice).sessionTicketLifetime
}

// expiringLRUSessionCache is a tls.ClientSessionCache implementation that uses an LRU caching
// strategy and expires tickets older than the configured TTL.
//
// Adapted from crypto/tls.lruSessionCache.
type expiringLRUSessionCache struct {
	sync.Mutex

	m        map[string]*list.Element
	q        *list.List
	capacity int
	keyTTL   time.Duration
}

type sessionCacheEntry struct {
	sessionKey string
	state      *tls.ClientSessionState
	expiration time.Time
}

// newExpiringLRUSessionCache returns an expiringLRUSessionCache. This cache uses an LRU caching
// strategy to stick to the input capacity. Additionally, keys older than the configured TTL will
// expire. If capacity is < 1, a default capacity is used instead.
func newExpiringLRUSessionCache(capacity int, keyTTL time.Duration) *expiringLRUSessionCache {
	const defaultSessionCacheCapacity = 64

	if capacity < 1 {
		capacity = defaultSessionCacheCapacity
	}
	return &expiringLRUSessionCache{
		m:        make(map[string]*list.Element),
		q:        list.New(),
		capacity: capacity,
		keyTTL:   keyTTL,
	}
}

// Put adds the provided (sessionKey, cs) pair to the cache. If cs is nil, the entry
// corresponding to sessionKey is removed from the cache instead.
func (c *expiringLRUSessionCache) Put(sessionKey string, cs *tls.ClientSessionState) {
	c.Lock()
	defer c.Unlock()

	if elem, ok := c.m[sessionKey]; ok {
		if cs == nil {
			c.delete(elem)
			return
		}
		entry := elem.Value.(*sessionCacheEntry)
		if !bytes.Equal(entry.state.SessionTicket(), cs.SessionTicket()) {
			// This is a new ticket, so update the expiration.
			entry.expiration = time.Now().Add(c.keyTTL)
		}
		entry.state = cs
		c.q.MoveToFront(elem)
		return
	}

	if c.q.Len() < c.capacity {
		entry := &sessionCacheEntry{sessionKey, cs, time.Now().Add(c.keyTTL)}
		c.m[sessionKey] = c.q.PushFront(entry)
		return
	}

	elem := c.q.Back()
	entry := elem.Value.(*sessionCacheEntry)
	delete(c.m, entry.sessionKey)
	entry.sessionKey = sessionKey
	entry.state = cs
	entry.expiration = time.Now().Add(c.keyTTL)
	c.q.MoveToFront(elem)
	c.m[sessionKey] = elem
}

// Get returns the ClientSessionState value associated with a given key. It
// returns (nil, false) if no value is found.
func (c *expiringLRUSessionCache) Get(sessionKey string) (*tls.ClientSessionState, bool) {
	c.Lock()
	defer c.Unlock()

	if elem, ok := c.m[sessionKey]; ok {
		entry := elem.Value.(*sessionCacheEntry)
		if entry.expiration.Before(time.Now()) {
			c.delete(elem)
			return nil, false
		}
		c.q.MoveToFront(elem)
		return entry.state, true
	}
	return nil, false
}

func (c *expiringLRUSessionCache) delete(elem *list.Element) {
	c.q.Remove(elem)
	delete(c.m, elem.Value.(*sessionCacheEntry).sessionKey)
}
