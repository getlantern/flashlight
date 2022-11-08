package redis

import (
	"sync"
	"sync/atomic"

	"github.com/getlantern/golog"
	"github.com/getlantern/msgpack"
	"github.com/go-redis/redis/v8"
)

var (
	log = golog.LoggerFor("goexpr.redis")

	redisClient atomic.Value
	caches      map[string]*cache
	cacheSize   int
	cacheMx     sync.Mutex
)

func init() {
	msgpack.RegisterExt(90, &hget{})
	msgpack.RegisterExt(91, &sismember{})
	msgpack.RegisterExt(92, &lua{})
}

func Configure(client *redis.Client, maxCacheSize int) {
	redisClient.Store(client)
	cacheMx.Lock()
	caches = make(map[string]*cache)
	cacheSize = maxCacheSize
	cacheMx.Unlock()
}

func getRedisClient() *redis.Client {
	_client := redisClient.Load()
	if _client == nil {
		return nil
	}
	return _client.(*redis.Client)
}
