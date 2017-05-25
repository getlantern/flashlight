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

type mockService struct {
	t       Type
	p       Publisher
	started bool
	stopped bool
}

func New1() Service {
	return &mockService{t: serviceType1}
}

func New2() Service {
	return &mockService{t: serviceType2}
}

func New3() Service {
	return &mockService{t: serviceType3}
}

func (i *mockService) GetType() Type {
	return i.t
}
func (i *mockService) Start() {
	i.started = true
	i.stopped = false
}
func (i *mockService) Stop() {
	i.started = false
	i.stopped = true
}

func (i *mockService) SetPublisher(p Publisher) {
	i.p = p
}

func TestRegister(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(New1(), nil)
	registry.MustRegister(New2(), nil)
	err := registry.Register(New2(), nil)
	assert.Error(t, err, "each service should be registered only once")
	i1 := registry.MustLookup(serviceType1)
	i2 := registry.MustLookup(serviceType2)
	registry.StartAll()
	assert.True(t, i1.(*mockService).started)
	assert.True(t, i2.(*mockService).started)
	registry.StopAll()
	assert.False(t, i1.(*mockService).started)
	assert.False(t, i2.(*mockService).started)
	assert.True(t, i1.(*mockService).stopped)
	assert.True(t, i2.(*mockService).stopped)
}

type mockWithPublisher struct {
	*mockService
	chStop chan bool
}

func (i *mockWithPublisher) Start() {
	i.mockService.Start()
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case ts := <-t.C:
				i.mockService.p.Publish(ts)
			case <-i.chStop:
				fmt.Print("stopping\n")
				return
			}
		}
	}()
}

func TestSubscribe(t *testing.T) {
	new1 := func() Service {
		return &mockWithPublisher{New1().(*mockService), make(chan bool)}
	}
	registry := NewRegistry()
	err := registry.Register(new1(), nil)
	assert.NoError(t, err)
	ch1 := registry.SubCh(serviceType1)
	ch2 := registry.SubCh(serviceType1)
	registry.Start(serviceType1)
	ts1 := <-ch1
	ts2 := <-ch2
	assert.Equal(t, ts1, ts2)
	ts1 = <-ch1
	ts2 = <-ch2
	registry.StopAll()
}
