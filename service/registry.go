package service

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/getlantern/errors"
)

// Registry registers, lookups, and manages the dependencies of all services.
type Registry struct {
	mu    sync.RWMutex
	nodes map[ID]*node

	// Note: we use separate mutex and map to avoid deadlock when publishing
	// message in Service.Start, which is useful in certain cases to publish
	// initial messages.
	muChannels sync.RWMutex
	channels   map[ID][]chan interface{}
}

type node struct {
	id       ID
	opts     ConfigOpts
	instance Service
	started  bool
}

// NewRegistry creates a new Registry
func NewRegistry() *Registry {
	return &Registry{
		nodes:    make(map[ID]*node),
		channels: make(map[ID][]chan interface{}),
	}
}

// MustRegister is same as Register but panics if fail to register the service.
func (r *Registry) MustRegister(instance Service, defaultOpts ConfigOpts) {
	err := r.Register(instance, defaultOpts)
	if err != nil {
		panic(err.Error())
	}
}

// Register register a service. It requires:
// 1. A method to create the service instance, typically New();
// 2. The default config options to start the service, or nil if the service
// doesn't need config.
// Registry.StartAll() will resolve the startup order.
func (r *Registry) Register(instance Service, defaultOpts ConfigOpts) error {
	if instance == nil {
		return errors.New("nil instance")
	}
	id := instance.GetID()
	if _, ok := instance.(Configurable); ok {
		if defaultOpts == nil {
			return fmt.Errorf("configurable service '%s' must be registered with default ConfigOpts", id)
		}
		if defaultOpts.For() != id {
			return fmt.Errorf("invalid default config options for %s", id)
		}
	} else if defaultOpts != nil {
		return fmt.Errorf("service '%s' is not configurable", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nodes[id] != nil {
		return fmt.Errorf("service '%s' is already registered", id)
	}
	r.nodes[id] = &node{id: id, instance: instance, opts: defaultOpts}
	if p, ok := instance.(WillPublish); ok {
		p.SetPublisher(publisher{id, r})
	}
	log.Debugf("Registered service %s", id)
	return nil
}

// MustLookup returns the service reference of id, or panics.
func (r *Registry) MustLookup(id ID) Service {
	if i := r.Lookup(id); i != nil {
		return i
	}
	panic(fmt.Sprintf("service id '%s' is not registered", string(id)))
}

// Lookup returns the service reference of id t, or nil if not found.
func (r *Registry) Lookup(id ID) Service {
	n := r.lookup(id)
	if n == nil {
		return nil
	}
	return n.instance
}

func (r *Registry) lookup(id ID) *node {
	r.mu.RLock()
	n := r.nodes[id]
	r.mu.RUnlock()
	return n
}

// MustConfigure configures the service, or panics.
func (r *Registry) MustConfigure(id ID, op func(ConfigOpts)) {
	if err := r.Configure(id, op); err != nil {
		panic(err)
	}
}

// Configure alters the ConfigOpts stored in the registry. When the ConfigOpts
// are complete, It passes the ConfigOpts to the service and starts it
// automatically. If the service is not configurable, it does nothing and
// returns error.
func (r *Registry) Configure(id ID, op func(ConfigOpts)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.nodes[id]
	if n == nil {
		return errors.New("%s service doesn't exist", id)
	}
	if n.opts == nil {
		return errors.New("%s service doesn't allow config options", id)
	}
	op(n.opts)
	r.startNoLock(n)
	return nil
}

// MustSubCh calls SubCh and panics if there's any error.
func (r *Registry) MustSubCh(id ID) <-chan interface{} {
	ch, err := r.SubCh(id)
	if err != nil {
		panic(err)
	}
	return ch
}

// SubCh returns a channel to receive any message the service published. The channel has 1 buffer in case , but messages are discarded if no one is listening on the channel. If the
// service doesn't implement WillPublish interface, the channel never sends
// anything. The channel will be closed by CloseAll().
func (r *Registry) SubCh(id ID) (<-chan interface{}, error) {
	r.mu.Lock()
	n := r.nodes[id]
	r.mu.Unlock()
	if n == nil {
		return nil, errors.New("%s service doesn't exist", id)
	}
	if _, ok := n.instance.(WillPublish); !ok {
		return nil, errors.New("%s service doesn't publish anything", id)
	}
	ch := make(chan interface{}, 1)
	r.muChannels.Lock()
	r.channels[id] = append(r.channels[id], ch)
	r.muChannels.Unlock()
	log.Tracef("Subscribed to %v", id)
	return ch, nil
}

// MustSub calls Sub and panics if there's any error.
func (r *Registry) MustSub(id ID, cb func(m interface{})) {
	if err := r.Sub(id, cb); err != nil {
		panic(err)
	}
}

// Sub calls SubCh with the the specific service id spawns a goroutine to
// call the callback for any messsage received.
func (r *Registry) Sub(id ID, cb func(m interface{})) error {
	ch, err := r.SubCh(id)
	if err != nil {
		return err
	}
	go func() {
		for m := range ch {
			cb(m)
		}
	}()

	return nil
}

func (r *Registry) publish(id ID, msg interface{}) {
	r.muChannels.RLock()
	defer r.muChannels.RUnlock()
	channels := r.channels[id]
	log.Tracef("Publishing message to %d subscribers of %v", len(channels), id)
	for _, ch := range channels {
		select {
		case ch <- msg:
		default:
			log.Debugf("Warning: message from %s discarded: %+v", id, msg)
		}
	}
}

// StartAll starts all the services unless any of the dependencies doesn't
// start.
func (r *Registry) StartAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, n := range r.nodes {
		r.startNoLock(n)
	}
}

type serviceWrapper struct {
	st   ID
	f    func() func()
	stop func()
}

func (s *serviceWrapper) GetID() ID {
	return s.st
}

func (s *serviceWrapper) Start() {
	s.stop = s.f()
}

func (s *serviceWrapper) Stop() {
	s.stop()
}

// StartFunc registers an anonymous service and starts it immediately. The
// service runs until CloseAll() is called.
func (r *Registry) StartFunc(f func() func()) {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	st := ID(name)
	r.Register(&serviceWrapper{st: st, f: f}, nil)
	r.Start(st)
}

// Start tries to start a service. If the service is configurable, the stored
// ConfigOpts are passed to the service before starting. Service is not started
// if the ConfigOpts are not complete. Starting an already started service is a
// no-op. The return value indicates whether the service is started or not
// after this function call.
func (r *Registry) Start(id ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := r.nodes[id]
	return r.startNoLock(n)
}

func (r *Registry) startNoLock(n *node) bool {
	if n.started {
		log.Debugf("Not start already started service %s", n.id)
		return true
	}
	if c, ok := n.instance.(Configurable); ok {
		if reason := n.opts.Complete(); reason != "" {
			log.Debugf("%s, skip starting service %s", reason, n.id)
			log.Tracef("%+v", n.opts)
			return false
		}
		c.Configure(n.opts)
	}
	log.Debugf("Starting service %s", n.id)
	n.instance.Start()
	n.started = true
	log.Debugf("Started service %s", n.id)
	return true
}

// CloseAll stops all services registered and started. It closes all channels
// subscribed to the services.
func (r *Registry) CloseAll() {
	r.stopServices()
	r.closeChannels()
}

func (r *Registry) stopServices() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, n := range r.nodes {
		r.stopNoLock(n)
	}
}

func (r *Registry) closeChannels() {
	r.muChannels.Lock()
	defer r.muChannels.Unlock()
	for _, channels := range r.channels {
		for _, ch := range channels {
			close(ch)
		}
	}
	// to avoid sending to closed channel
	r.channels = make(map[ID][]chan interface{})
}

// Stop stops a service but does nothing if the service was not already started.
func (r *Registry) Stop(id ID) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := r.nodes[id]
	if n == nil {
		log.Errorf("Stopping not registered service %s", id)
	}
	r.stopNoLock(n)
}

func (r *Registry) stopNoLock(n *node) {
	if !n.started {
		return
	}
	n.instance.Stop()
	n.started = false
	log.Debugf("Stopped service %s", n.id)
}
