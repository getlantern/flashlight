package redis

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru"
)

const (
	defaultCacheInvalidationPeriod = 15 * time.Minute
)

var (
	cacheInvalidationPeriod int64 = int64(defaultCacheInvalidationPeriod)
)

func SetCacheInvalidationPeriod(period time.Duration) {
	atomic.StoreInt64(&cacheInvalidationPeriod, int64(period))
}

func getCacheInvalidationPeriod() time.Duration {
	return time.Duration(atomic.LoadInt64(&cacheInvalidationPeriod))
}

type cache struct {
	*lru.Cache
	key string
	mx  sync.RWMutex
}

func cacheFor(key string, maxSize int) *cache {
	cacheMx.Lock()
	result, found := caches[key]
	if !found {
		c, _ := lru.New(maxSize)
		result = &cache{
			Cache: c,
			key:   key,
		}
		go result.invalidate()
		caches[key] = result
	}
	cacheMx.Unlock()
	return result
}

func (c *cache) Get(key interface{}) (interface{}, bool) {
	c.mx.RLock()
	result, found := c.Cache.Get(key)
	c.mx.RUnlock()
	return result, found
}

func (c *cache) invalidate() {
	for {
		time.Sleep(getCacheInvalidationPeriod())
		log.Debugf("Clearing cache for '%v'", c.key)
		c.mx.Lock()
		c.Purge()
		c.mx.Unlock()
	}
}
