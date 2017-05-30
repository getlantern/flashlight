package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var st1 = ID("service.id1")
var st2 = ID("service.id2")
var st3 = ID("service.id3")

type mockService struct {
	t          ID
	startCount int
	stopCount  int
}

func (i *mockService) GetID() ID {
	return i.t
}

func (i *mockService) Start() {
	i.startCount++
}

func (i *mockService) Stop() {
	i.stopCount++
}

type mockOpts struct {
	t ID
	s string
}

func (o *mockOpts) For() ID {
	return o.t
}

func (o *mockOpts) Complete() string {
	return o.s
}

func TestLifeCycle(t *testing.T) {
	reg := NewRegistry()
	err := reg.Register(nil, nil)
	assert.Error(t, err, "registering nil service should fail")
	reg.MustRegister(&mockService{t: st1}, nil)
	err = reg.Register(&mockService{t: st1}, nil)
	assert.Error(t, err, "each service should be registered only once")
	err = reg.Register(&mockService{t: st2}, &mockOpts{st2, ""})
	assert.Error(t, err, "registering non-configurable service with opts should fail")
	reg.MustRegister(&mockService{t: st2}, nil)
	i1 := reg.MustLookup(st1).(*mockService)
	i2 := reg.MustLookup(st2).(*mockService)
	reg.Start(st1)
	assert.Equal(t, 1, i1.startCount)
	assert.Equal(t, 0, i2.startCount)
	reg.StartAll()
	assert.Equal(t, 1, i1.startCount, "should start only once")
	assert.Equal(t, 1, i2.startCount)
	reg.Stop(st1)
	assert.Equal(t, 1, i1.stopCount)
	assert.Equal(t, 0, i2.stopCount)
	reg.CloseAll()
	assert.Equal(t, 1, i1.stopCount, "should stop only once")
	assert.Equal(t, 1, i2.stopCount)
}

type mockWithPublisher struct {
	*mockService
	p      Publisher
	chStop chan bool
}

func (i *mockWithPublisher) Start() {
	i.mockService.Start()
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case ts := <-t.C:
				i.p.Publish(ts)
			case <-i.chStop:
				fmt.Print("stopping\n")
				return
			}
		}
	}()
}

func (i *mockWithPublisher) SetPublisher(p Publisher) {
	i.p = p
}

func TestSubscribe(t *testing.T) {
	reg := NewRegistry()
	inst := &mockWithPublisher{&mockService{t: st1}, nil, make(chan bool)}
	reg.MustRegister(inst, nil)
	reg.MustRegister(&mockService{t: st2}, nil)
	_, err := reg.SubCh(st3)
	assert.Error(t, err, "subscribing to not registered service should fail")
	_, err = reg.SubCh(st2)
	assert.Error(t, err, "subscribing should fail if service doesn't publish")
	ch1 := reg.MustSubCh(st1)
	ch2 := reg.MustSubCh(st1)
	reg.Start(st1)
	ts1 := <-ch1
	ts2 := <-ch2
	assert.Equal(t, ts1, ts2, "should receive ts from both channels")
	ts1 = <-ch1
	ts2 = <-ch2
	assert.Equal(t, ts1, ts2, "should receive ts again from both channels")
	reg.CloseAll()
	ts1 = <-ch1
	ts2 = <-ch1
	assert.Nil(t, ts1, "CloseAll() should have closed all channels")
	assert.Nil(t, ts2, "CloseAll() should have closed all channels")
}
