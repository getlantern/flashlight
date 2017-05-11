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
func (i *mockImpl) Reconfigure(p Publisher, opts ConfigOpts) {
	i.p = p
	i.opts = opts
}

func TestRegister(t *testing.T) {
	registry := NewRegistry()
	_, _, err := registry.Register(New1, nil, true, []Type{serviceType2})
	assert.Error(t, err,
		"should not register if any of the dependencies is not found")
	registry.MustRegister(New1, nil, true, nil)
	registry.MustRegister(New2, nil, true, []Type{serviceType1})
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
	_, i2 := registry.MustRegister(New2, nil, false, []Type{serviceType1})
	_, i3 := registry.MustRegister(New3, nil, true, []Type{serviceType2})
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

type mockMessage time.Time

func (m mockMessage) ValidMessageFrom(t Type) bool {
	return t == serviceType1
}

func (i *mockWithPublisher) Start() {
	i.mockImpl.Start()
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case ts := <-t.C:
				i.mockImpl.p.Publish(mockMessage(ts))
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
	ch1 := s1.Subscribe()
	ch2 := s1.Subscribe()
	s1.Start()
	ts1 := <-ch1
	ts2 := <-ch2
	assert.Equal(t, ts1, ts2)
	ts1 = <-ch1
	ts2 = <-ch2
	registry.StopAll()
}

type mockConfigOpts struct {
	OptInt    int
	OptBool   bool
	OptString string
	OptFunc   func() bool
	OptStruct struct {
		A int
		B string
	}
}

func (o *mockConfigOpts) ValidConfigOptsFor(t Type) bool {
	return t == serviceType1
}

func TestReconfigure(t *testing.T) {
	s1, i1 := NewRegistry().MustRegister(New1, &mockConfigOpts{}, true, nil)
	err := s1.Reconfigure(ConfigUpdates{
		"OptInt":      1,
		"OptString":   "abc",
		"OptBool":     true,
		"OptFunc":     func() bool { return true },
		"OptStruct.A": 1,
		"OptStruct.B": "cde",
	})
	if assert.NoError(t, err) {
		opts := i1.(*mockImpl).opts.(*mockConfigOpts)
		assert.Equal(t, 1, opts.OptInt)
		assert.Equal(t, "abc", opts.OptString)
		assert.Equal(t, true, opts.OptBool)
		assert.Equal(t, true, opts.OptFunc())
		assert.Equal(t, 1, opts.OptStruct.A)
		assert.Equal(t, "cde", opts.OptStruct.B)
	}
	err = s1.Reconfigure(ConfigUpdates{"OptNotExist": 1})
	assert.Error(t, err)
	err = s1.Reconfigure(ConfigUpdates{"OptString": 1})
	assert.Error(t, err)
	err = s1.Reconfigure(ConfigUpdates{"OptNotExistStruct.C": 1})
	assert.Error(t, err)
}
