package chained

import (
	"bytes"
	"container/list"
	"sort"
	"sync"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/geolookup"
	tls "github.com/refraction-networking/utls"
)

// We look up the client's geolocation to determine which browser to emulate.
const geoLookupTimeout = 2 * time.Second

type browserData struct {
	sessionTicketLifetime time.Duration
}

type regionalBrowserData struct {
	browserData
	marketShare float64
}

var (
	// We treat Internet Explorer and Edge as the same thing below.

	// https://github.com/getlantern/lantern-internal/issues/3315#issue-560602994
	chrome     = browserData{30 * time.Minute}
	safari     = browserData{24 * time.Hour}
	firefox    = browserData{24 * time.Hour}
	edge       = browserData{10 * time.Hour}
	threeSixty = browserData{9 * time.Hour}
	qq         = browserData{30 * time.Minute}

	// https://gs.statcounter.com/browser-market-share#monthly-201910-201910-bar
	globalBrowsers = []regionalBrowserData{
		{chrome, 0.65},
		{safari, 0.17},
		{firefox, 0.04},
		{edge, 0.04},
	}

	// https://github.com/getlantern/lantern-internal/issues/3315#issuecomment-589253390
	browsersByCountry = map[string][]regionalBrowserData{
		"CN": []regionalBrowserData{
			{edge, 0.36},
			{threeSixty, 0.26},
			{qq, 0.10},
			{firefox, 0.03},
		},
	}
)

// Chooses a TTL for session tickets for this client. This decision is made using data on market
// share for the top 4 browsers and the session ticket lifetimes enforced by those browsers.
//
// If necessary, we use region-specific market share figures. This is based on the client's
// geolocation and thus this function may block for a period while geolocation is determined.
func chooseSessionTicketTTL(uc common.UserConfig) time.Duration {
	log.Debugf("[%v] entering chooseSessionTicketTTL; looking up country", time.Now())

	countryCode := geolookup.GetCountry(geoLookupTimeout)
	log.Debugf("[%v] completed country code lookup", time.Now())
	if countryCode == "" {
		log.Error("failed to retrieve country code; using default session ticket lifetime settings")
	}
	browsers, ok := browsersByCountry[countryCode]
	if !ok {
		browsers = globalBrowsers
	}
	browsersToWeights := map[int]int{}
	for i, b := range browsers {
		browsersToWeights[i] = int(b.marketShare * 100)
	}
	chosenBrowser := chooseBucketForUser(uc.GetUserID(), browsersToWeights)
	return browsers[chosenBrowser].sessionTicketLifetime
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

// Choose a bucket for the user with the input user ID.
//
// The idea is to deterministically place users into buckets such that a given user is always placed
// into the same bucket. The inputs must be the same every time to guarantee determinism.
func chooseBucketForUser(userID int64, bucketsToWeights map[int]int) int {
	type weightedBucket struct {
		index, weight int
	}

	buckets := []weightedBucket{}
	weightTotal := 0
	for bucket, weight := range bucketsToWeights {
		buckets = append(buckets, weightedBucket{bucket, weight})
		weightTotal += weight
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].weight < buckets[j].weight })

	if userID < 0 {
		userID *= -1
	}
	choice := userID % int64(weightTotal)
	for _, b := range buckets {
		if choice < int64(b.weight) {
			return b.index
		}
		choice -= int64(b.weight)
	}
	// Shouldn't be possible to reach this point unless bucketsToWeights is empty.
	return 0
}
