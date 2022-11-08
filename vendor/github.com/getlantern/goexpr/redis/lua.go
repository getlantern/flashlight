package redis

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/getlantern/goexpr"
)

var (
	scriptCache   = make(map[string]string, 0)
	scriptCacheMx sync.Mutex
)

type lua struct {
	Script goexpr.Expr
	Keys   []goexpr.Expr
	Args   []goexpr.Expr
}

func Lua(script goexpr.Expr, keys []goexpr.Expr, args ...goexpr.Expr) goexpr.Expr {
	return &lua{
		Script: script,
		Keys:   keys,
		Args:   args,
	}
}

func (e *lua) Eval(params goexpr.Params) interface{} {
	redisClient := getRedisClient()

	_script := e.Script.Eval(params)
	if _script == nil {
		return nil
	}
	script := _script.(string)

	keys := make([]string, 0, len(e.Keys))
	args := make([]interface{}, 0, len(e.Args))
	valueKey := ""
	for _, keyEx := range e.Keys {
		key := keyEx.Eval(params).(string)
		keys = append(keys, key)
		valueKey = fmt.Sprintf("%v|%v", valueKey, key)
	}
	for _, argEx := range e.Args {
		arg := argEx.Eval(params)
		args = append(args, arg)
		valueKey = fmt.Sprintf("%v|%v", valueKey, arg)
	}

	scriptCacheMx.Lock()
	sha := scriptCache[script]
	if sha == "" {
		var loadErr error
		sha, loadErr = redisClient.ScriptLoad(context.Background(), script).Result()
		if loadErr != nil {
			log.Errorf("Unable to load lua script '%v': %v", script, loadErr)
			return nil
		}
		scriptCache[script] = sha
	}
	scriptCacheMx.Unlock()

	// Check cache
	cacheKey := fmt.Sprintf("|||||||||script*********%v", script)
	cache := cacheFor(cacheKey, cacheSize)
	cached, cachedFound := cache.Get(valueKey)
	if cachedFound {
		return cached
	}

	value, err := redisClient.EvalSha(context.Background(), sha, keys, args...).Result()
	if err != nil {
		log.Errorf("Error evaluating script '%v': %v", script, err)
		return nil
	}
	cache.Add(valueKey, value)
	return value
}

func (e *lua) WalkParams(cb func(string)) {
	e.Script.WalkParams(cb)
	for _, key := range e.Keys {
		key.WalkParams(cb)
	}
	for _, arg := range e.Args {
		arg.WalkParams(cb)
	}
}

func (e *lua) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *lua) WalkLists(cb func(goexpr.List)) {
	e.Script.WalkLists(cb)
	for _, key := range e.Keys {
		key.WalkLists(cb)
	}
	for _, arg := range e.Args {
		arg.WalkLists(cb)
	}
}

func (e *lua) String() string {
	keys := make([]string, 0, len(e.Keys))
	for _, key := range e.Keys {
		keys = append(keys, key.String())
	}
	args := make([]string, 0, len(e.Args))
	for _, arg := range e.Args {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("LUA(%v,%v,%v)", e.Script, strings.Join(keys, ","), strings.Join(args, ","))
}
