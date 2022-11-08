package redis

import (
	"context"
	"fmt"

	"github.com/getlantern/goexpr"
)

type sismember struct {
	Key    goexpr.Expr
	Member goexpr.Expr
}

func SIsMember(key goexpr.Expr, member goexpr.Expr) goexpr.Expr {
	return &sismember{
		Key:    key,
		Member: member,
	}
}

func (e *sismember) Eval(params goexpr.Params) interface{} {
	redisClient := getRedisClient()

	_key := e.Key.Eval(params)
	if _key == nil {
		return nil
	}
	key := _key.(string)
	_member := e.Member.Eval(params)
	if _member == nil {
		return nil
	}
	member := fmt.Sprint(_member)

	// Check cache
	cache := cacheFor(key, cacheSize)
	cached, cachedFound := cache.Get(member)
	if cachedFound {
		return cached
	}

	value, _ := redisClient.SIsMember(context.Background(), key, member).Result()
	cache.Add(member, value)
	return value
}

func (e *sismember) WalkParams(cb func(string)) {
	e.Key.WalkParams(cb)
	e.Member.WalkParams(cb)
}

func (e *sismember) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *sismember) WalkLists(cb func(goexpr.List)) {
	e.Key.WalkLists(cb)
}

func (e *sismember) String() string {
	return fmt.Sprintf("SISMEMBER(%v,%v)", e.Key, e.Member)
}
