package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var serviceType1 = Type("service.type1")
var serviceType2 = Type("service.type2")
var serviceType3 = Type("service.type3")

type mockImpl struct {
	t       Type
	p       Publisher
	started bool
	stopped bool
	opts    ConfigOpts
}

func New1() Impl {
	return &mockImpl{t: serviceType1}
}

func New2() Impl {
	return &mockImpl{t: serviceType2}
}

func New3() Impl {
	return &mockImpl{t: serviceType3}
}

func (i *mockImpl) GetType() Type {
	return i.t
}
func (i *mockImpl) Start() {
	i.started = true
	i.stopped = false
}
func (i *mockImpl) Stop() {
	i.started = false
	i.stopped = true
}
func (i *mockImpl) Reconfigure(opts ConfigOpts) {
	i.opts = opts
}

func (i *mockImpl) SetPublisher(p Publisher) {
	i.p = p
}

func TestRegister(t *testing.T) {
	registry := NewRegistry()
	_, _, err := registry.Register(New1, nil, true, Deps{serviceType2: nil})
	assert.Error(t, err,
		"should not register if any of the dependencies is not found")
	registry.MustRegister(New1, nil, true, nil)
	registry.MustRegister(New2, nil, true, Deps{serviceType1: nil})
	_, _, err = registry.Register(New2, nil, true, nil)
	assert.Error(t, err, "each service should be registered only once")
	_, i1 := registry.MustLookup(serviceType1)
	_, i2 := registry.MustLookup(serviceType2)
	registry.StartAll()
	assert.True(t, i1.(*mockImpl).started)
	assert.True(t, i2.(*mockImpl).started)
	registry.StopAll()
	assert.False(t, i1.(*mockImpl).started)
	assert.False(t, i2.(*mockImpl).started)
	assert.True(t, i1.(*mockImpl).stopped)
	assert.True(t, i2.(*mockImpl).stopped)
}

func TestAutoStart(t *testing.T) {
	registry := NewRegistry()
	_, i1 := registry.MustRegister(New1, nil, true, nil)
	_, i2 := registry.MustRegister(New2, nil, false, Deps{serviceType1: nil})
	_, i3 := registry.MustRegister(New3, nil, true, Deps{serviceType2: nil})
	registry.StartAll()
	assert.True(t, i1.(*mockImpl).started)
	assert.False(t, i2.(*mockImpl).started,
		"should not auto start if autoStart is false")
	assert.False(t, i3.(*mockImpl).started,
		"should not auto start if one of the dependencies doesn't auto start")
	registry.StopAll()
}

type mockWithPublisher struct {
	*mockImpl
	chStop chan bool
}

func (i *mockWithPublisher) Start() {
	i.mockImpl.Start()
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case ts := <-t.C:
				i.mockImpl.p.Publish(ts)
			case <-i.chStop:
				fmt.Print("stopping\n")
				return
			}
		}
	}()
}

func TestSubscribe(t *testing.T) {
	new1 := func() Impl {
		return &mockWithPublisher{New1().(*mockImpl), make(chan bool)}
	}
	registry := NewRegistry()
	s1, _, err := registry.Register(new1, nil, true, nil)
	assert.NoError(t, err)
	ch1 := registry.Subscribe(serviceType1)
	ch2 := registry.Subscribe(serviceType1)
	s1.Start()
	ts1 := <-ch1
	ts2 := <-ch2
	assert.Equal(t, ts1, ts2)
	ts1 = <-ch1
	ts2 = <-ch2
	registry.StopAll()
}
