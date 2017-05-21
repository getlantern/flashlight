package service

import (
	"fmt"
	"sync"

	"github.com/getlantern/errors"
)

// Registry registers, lookups, and manages the dependencies of all services.
type Registry struct {
	muDag      sync.RWMutex
	dag        *dag
	muChannels sync.RWMutex
	channels   map[Type][]chan interface{}
}

// NewRegistry creates a new Registry
func NewRegistry() *Registry {
	return &Registry{
		dag:      newDag(),
		channels: make(map[Type][]chan interface{}),
	}
}

// MustRegister is same as Register but panics if fail to register the service.
func (r *Registry) MustRegister(
	instance Impl,
	defaultOpts ConfigOpts,
	deps Deps) (Service, Impl) {
	s, i, err := r.Register(instance, defaultOpts, deps)
	if err != nil {
		panic(err.Error())
	}
	return s, i
}

// Register register a service. It requires:
// 1. A method to create the service instance, typically New();
// 2. The default config options to start the service, or nil if the service
// doesn't need config.
// 3. Whether start the service when calling Registry.StartAll().
// 4. A set of services on which it depends, or nil if no dependence at all.
// Registry.StartAll() will resolve the startup order.
func (r *Registry) Register(
	instance Impl,
	defaultOpts ConfigOpts,
	deps Deps) (Service, Impl, error) {
	if instance == nil {
		return nil, nil, errors.New("nil instance")
	}
	t := instance.GetType()
	if _, ok := instance.(Configurable); ok {
		if defaultOpts == nil {
			return nil, nil, fmt.Errorf("Configurable service '%s' must be registered with default ConfigOpts", t)
		}
	}
	r.muDag.Lock()
	defer r.muDag.Unlock()
	if r.dag.Lookup(t) != nil {
		return nil, nil, fmt.Errorf("service '%s' is already registered", t)
	}
	if defaultOpts != nil && defaultOpts.For() != t {
		return nil, nil, fmt.Errorf("invalid default config options type for %s", t)
	}
	for dt, _ := range deps {
		node := r.dag.Lookup(dt)
		if node == nil {
			return nil, nil, fmt.Errorf("service '%s' depends on not-registered service '%s'", t, dt)
		}
	}
	r.dag.AddVertex(t, instance, defaultOpts)
	s := service{instance, r}
	if p, ok := instance.(WillPublish); ok {
		p.SetPublisher(publisher{t, r})
	}
	for dt, df := range deps {
		r.dag.AddEdge(dt, t)
		if df != nil {
			ch := r.Subscribe(dt)
			go func() {
				for m := range ch {
					df(m, s)
				}
			}()
		}
	}
	log.Debugf("Registered service %s", t)
	return s, instance, nil
}

// MustLookup returns the service reference of type t, or panics.
func (r *Registry) MustLookup(t Type) (Service, Impl) {
	if s, i := r.Lookup(t); s != nil {
		return s, i
	}
	panic(fmt.Sprintf("service type '%s' is not registered", string(t)))
}

// Lookup returns the service reference of type t, or nil if not found.
func (r *Registry) Lookup(t Type) (Service, Impl) {
	n := r.lookup(t)
	if n == nil {
		return nil, nil
	}
	return service{n.instance, r}, n.instance
}

func (r *Registry) lookup(t Type) *node {
	r.muDag.RLock()
	n := r.dag.Lookup(t)
	r.muDag.RUnlock()
	return n
}

// StartAll starts all the services unless any of the dependencies doesn't
// start.
func (r *Registry) StartAll() {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	for _, n := range r.dag.Flatten() {
		r.startNoLock(n)
	}
}

func (r *Registry) started(t Type) bool {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	n := r.dag.Lookup(t)
	return n.started
}

func (r *Registry) start(t Type) bool {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	n := r.dag.Lookup(t)
	return r.startNoLock(n)
}

// TODO: enforce timeout
func (r *Registry) startNoLock(n *node) bool {
	if n.started {
		log.Debugf("Not start already started service %s", n.t)
		return true
	}
	if c, ok := n.instance.(Configurable); ok {
		if reason := n.opts.Complete(); reason != "" {
			log.Debugf("%s, skip starting service %s", reason, n.t)
			log.Tracef("%+v", n.opts)
			return false
		}
		c.Configure(n.opts)
	}
	log.Debugf("Starting service %s", n.t)
	n.instance.Start()
	n.started = true
	log.Debugf("Started service %s", n.t)
	return true
}

// StopAll stops all services registered and started. It closes all channels subscribed to the services.
func (r *Registry) StopAll() {
	r.stopServices()
	r.closeChannels()
}

func (r *Registry) stopServices() {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	flatten := r.dag.Flatten()
	// Stop in reverse order
	for i := len(flatten) - 1; i >= 0; i-- {
		r.stopNoLock(flatten[i])
	}
}

func (r *Registry) closeChannels() {
	r.muChannels.Lock()
	allChannels := r.channels
	r.channels = make(map[Type][]chan interface{})
	r.muChannels.Unlock()
	for _, channels := range allChannels {
		for _, ch := range channels {
			close(ch)
		}
	}
}

func (r *Registry) stop(t Type) {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	n := r.dag.Lookup(t)
	r.stopNoLock(n)
}

// TODO: enforce timeout
func (r *Registry) stopNoLock(n *node) {
	if n.started {
		n.instance.Stop()
		n.started = false
		log.Debugf("Stopped service %s", n.t)
	}
}

func (r *Registry) MustConfigure(t Type, op func(ConfigOpts)) {
	if err := r.Configure(t, op); err != nil {
		panic(err)
	}
}

// TODO: enforce timeout
func (r *Registry) Configure(t Type, op func(ConfigOpts)) error {
	r.muDag.Lock()
	defer r.muDag.Unlock()
	n := r.dag.Lookup(t)
	if n.opts == nil {
		return errors.New("%s service doesn't allow config options", t)
	}
	op(n.opts)
	r.startNoLock(n)
	return nil
}

// Subscribe returns a channel to receive any message the service published.
// Messages are discarded if no one is listening on the channel.
// If the service doesn't implement WillPublish interface, the channel never
// sends anything.
func (r *Registry) Subscribe(t Type) <-chan interface{} {
	ch := make(chan interface{}, 10)
	r.muChannels.Lock()
	r.channels[t] = append(r.channels[t], ch)
	r.muChannels.Unlock()
	log.Tracef("Subscribed to %v", t)
	return ch
}

func (r *Registry) publish(t Type, msg interface{}) {
	r.muChannels.RLock()
	channels := r.channels[t]
	r.muChannels.RUnlock()
	log.Tracef("Publishing message to %d subscribers of %v", len(channels), t)
	for _, ch := range channels {
		select {
		case ch <- msg:
		default:
			log.Errorf("Warning: message from %s discarded: %+v", t, msg)
		}
	}
}
