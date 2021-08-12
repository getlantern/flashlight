package util

import (
	"net/url"
	"sync"
)

// DynamicEndpoint is a url.URL that can update it's own value based on a
// function
type DynamicEndpoint struct {
	val        *url.URL
	updateChan chan bool
	mutex      sync.RWMutex
}

// NewDynamicEndpoint returns a new DynamicEndpoint that's assigned a default
// value ('defaultEndpoint').
//
// A function is run to dynamically update the endpoint's value
// ('newEndpointGetter') based on a condition ('conditionChan').
//
// For testing, it's useful to have a channel to send errors to ('errChan').
func NewDynamicEndpoint(
	defaultEndpoint string,
	conditionChan <-chan bool,
	errChan chan error,
	announceUpdates bool,
	newEndpointGetter func() (string, error),
) (*DynamicEndpoint, error) {
	defaultEndpointUrl, err := url.Parse(defaultEndpoint)
	if err != nil {
		return nil, err
	}
	dynamicEndpoint := &DynamicEndpoint{
		val: defaultEndpointUrl,
	}
	if announceUpdates {
		dynamicEndpoint.updateChan = make(chan bool, 100)
	}
	if newEndpointGetter != nil {
		go func() {
			for {
				<-conditionChan
				newEndpoint, err := newEndpointGetter()
				if err != nil {
					if errChan != nil {
						errChan <- err
					}
					continue
				}
				newUrl, err := url.Parse(newEndpoint)
				if err != nil {
					if errChan != nil {
						errChan <- err
					}
					continue
				}
				// Set the new value
				dynamicEndpoint.set(newUrl)
				// Send a signal on the update channel, if not nil
				if dynamicEndpoint.updateChan != nil {
					dynamicEndpoint.updateChan <- true
				}
			}
		}()
	}
	return dynamicEndpoint, nil
}

// NewConstantDynamicEndpoint returns a DynamicEndpoint that never changes its value.
// This is only useful in places where a DynamicEndpoint is needed, but the
// endpoint never changes its value
func NewConstantDynamicEndpoint(endpoint string) (*DynamicEndpoint, error) {
	return NewDynamicEndpoint(endpoint, nil, nil, false, nil)
}

func (self *DynamicEndpoint) set(newVal *url.URL) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.val = newVal
}

func (self *DynamicEndpoint) Get() *url.URL {
	self.mutex.RLock()
	defer self.mutex.RUnlock()
	return self.val
}

func (self *DynamicEndpoint) OnUpdate() <-chan bool {
	return self.updateChan
}
