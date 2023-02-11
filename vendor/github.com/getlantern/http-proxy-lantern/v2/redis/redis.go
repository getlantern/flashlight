package redis

import (
	"crypto/tls"
	"fmt"

	"github.com/go-redis/redis/v8"
)

// Creates a new redis client with the specified redis URL to use, in the form:
// rediss://:password@host
func NewClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL")
	}

	return redis.NewClient(&redis.Options{
		Addr:     opt.Addr,
		Password: opt.Password,
		PoolSize: 2,
		TLSConfig: &tls.Config{
			ClientSessionCache: tls.NewLRUClientSessionCache(20),
		},
	}), nil
}
