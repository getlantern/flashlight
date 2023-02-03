package eventual

import (
	"context"
	"sync"
)

// DontWait is an expired context for use in Value.Get. Using DontWait will cause a Value.Get call
// to return immediately. If the value has not been set, a context.Canceled error will be returned.
var DontWait context.Context

func init() {
	var cancel func()
	DontWait, cancel = context.WithCancel(context.Background())
	cancel()
}

// Value is an eventual value, meaning that callers wishing to access the value block until it is
// available.
type Value interface {
	// Set this Value.
	Set(interface{})

	// Reset clears the currently set value, reverting to the same state as if the Eventual had just
	// been created.
	Reset()

	// Get waits for the value to be set. If the context expires first, an error will be returned.
	//
	// This function will return immediately when called with an expired context. In this case, the
	// value will be returned only if it has already been set; otherwise the context error will be
	// returned. For convenience, see DontWait.
	Get(context.Context) (interface{}, error)
}

// NewValue creates a new value.
func NewValue() Value {
	return WithDefault(nil)
}

// WithDefault creates a new value that returns the given defaultValue if a real value isn't
// available in time.
func WithDefault(defaultValue interface{}) Value {
	return &value{defaultValue: defaultValue}
}

type value struct {
	m            sync.Mutex
	v            interface{}
	defaultValue interface{}
	set          bool
	waiters      []chan interface{}
}

func (v *value) Set(i interface{}) {
	v.m.Lock()
	v.v = i
	if !v.set {
		// This is our first time setting, inform anyone who is waiting
		for _, waiter := range v.waiters {
			waiter <- i
		}
		v.waiters = make([]chan interface{}, 0)
		v.set = true
	}
	v.m.Unlock()
}

func (v *value) Reset() {
	v.m.Lock()
	v.v = nil
	v.set = false
	v.m.Unlock()
}

func (v *value) Get(ctx context.Context) (interface{}, error) {
	v.m.Lock()
	if v.set {
		// Value already set, use existing
		_v := v.v
		v.m.Unlock()
		return _v, nil
	}

	// Value not yet set, wait
	waiter := make(chan interface{}, 1)
	v.waiters = append(v.waiters, waiter)
	v.m.Unlock()
	select {
	case _v := <-waiter:
		return _v, nil
	case <-ctx.Done():
		if v.defaultValue != nil {
			return v.defaultValue, nil
		}
		return nil, ctx.Err()
	}
}
