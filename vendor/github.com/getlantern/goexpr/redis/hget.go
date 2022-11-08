package redis

import (
	"context"
	"fmt"

	"github.com/getlantern/goexpr"
)

type hget struct {
	Key   goexpr.Expr
	Field goexpr.Expr
}

func HGet(key goexpr.Expr, field goexpr.Expr) goexpr.Expr {
	return &hget{
		Key:   key,
		Field: field,
	}
}

func (e *hget) Eval(params goexpr.Params) interface{} {
	redisClient := getRedisClient()

	_key := e.Key.Eval(params)
	if _key == nil {
		return nil
	}
	key := _key.(string)
	_field := e.Field.Eval(params)
	if _field == nil {
		return nil
	}
	field := fmt.Sprint(_field)

	// Check cache
	cache := cacheFor(key, cacheSize)
	cached, cachedFound := cache.Get(field)
	if cachedFound {
		return cached
	}

	value, _ := redisClient.HGet(context.Background(), key, field).Result()
	cache.Add(field, value)
	return value
}

func (e *hget) WalkParams(cb func(string)) {
	e.Key.WalkParams(cb)
	e.Field.WalkParams(cb)
}

func (e *hget) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *hget) WalkLists(cb func(goexpr.List)) {
	e.Key.WalkLists(cb)
}

func (e *hget) String() string {
	return fmt.Sprintf("HGET(%v,%v)", e.Key, e.Field)
}
