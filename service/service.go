// Package service provides mechanism and interfaces to declare, register,
// lookup, and manage the lifecycle of a group of services, i.e., long-running
// tasks.
//
package service

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/getlantern/errors"
)

// Type uniquely identify a service. Typically, each service defines a package
// level constant ServiceType with an unique string.
type Type string

// ConfigUpdates represents a portion of the config options of a service. See
// Service.Reconfigure() for details.
type ConfigUpdates map[string]interface{}

// ConfigOpts represents all of the config options required to start a service.
type ConfigOpts interface {
	// ValidConfigOptsFor is called by Registry to check if the opts is for the
	// specific service and complete to start the service.
	ValidConfigOptsFor(t Type) bool
}

// Message represents anything a service wants to update with the rest of the
// world. Call Service.Subscribe() for any messages. service implementations
// call Publisher.Publish to broadcast the message to all subscribers.
type Message interface {
	// ValidMessageFrom is called by Registry to make sure the message is
	// received from the correct service.
	ValidMessageFrom(t Type) bool
}

// Service is the reference to a service. The only way to obtain such a
// reference is by calling Registry.Lookup().
type Service interface {
	// Start checks if the effective config options is valid, reconfigure the
	// service with it and starts the service.  It returns if the service is
	// currently started.  Starting an already started service is a noop and
	// returns true.
	Start() bool
	// Started returns true if service is started, false otherwise.
	Started() bool
	// Stop stops a started service. Stopping an unstarted service is a noop.
	Stop()
	// Reconfigure updates part of the effective config options. If the option
	// is valid, it calls Reconfigure() of the Impl and start the service if
	// not already started.
	//
	// The key of the map is the full path to the field to update, e.g., to
	// update `Bar` in `struct Opts { Foo: struct { Bar int } }`, key should be
	// `Foo.Bar`.  It returns error without doing anything if the field doesn't
	// exist or type mismatches. It's up to the Impl to restart itself when
	// required (service status is not affected).
	Reconfigure(fields ConfigUpdates) error
	// Subscribe gets a channel to receive any message the service published.
	// Messages are discarded if no one is listening on the channel.
	Subscribe() <-chan Message
	// GetImpl gets the implementation of the service. Caller usually casts it
	// to a concrete type to call its specific methods. Be aware that one
	// should always Start(), Stop(), or Reconfigure() via the Service, instead
	// of the Impl.
	GetImpl() Impl
}

// Publisher is an interface the service impletation to publish a message, when required.
type Publisher interface {
	// Publish publishes the message to all of the subscribers.
	Publish(Message)
}

// Impl actually implemetents the service.
type Impl interface {
	// GetType returns the type of the service
	GetType() Type
	// Start actually starts the service
	Start()
	// Stop actually stops the service
	Stop()
	// Reconfigure configures the service with current effective config options.
	Reconfigure(p Publisher, opts ConfigOpts)
}

// Registry registers, lookups, and manages the dependencies of all services.
type Registry struct {
	muDag      sync.RWMutex
	dag        *dag
	muChannels sync.RWMutex
	channels   map[Type][]chan Message
}

var singleton *Registry

func init() {
	singleton = NewRegistry()
}

// NewRegistry creates a new Registry
func NewRegistry() *Registry {
	return &Registry{
		dag:      newDag(),
		channels: make(map[Type][]chan Message),
	}
}

// GetRegistry gets the singleton registry
func GetRegistry() *Registry {
	return singleton
}

// MustRegister is same as Register but panics if fail to register the service.
func (r *Registry) MustRegister(
	instantiator func() Impl,
	defaultOpts ConfigOpts,
	autoStart bool,
	deps []Type) (Service, Impl) {
	s, i, err := r.Register(instantiator, defaultOpts, autoStart, deps)
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
	instantiator func() Impl,
	defaultOpts ConfigOpts,
	autoStart bool,
	deps []Type) (Service, Impl, error) {

	instance := instantiator()
	t := instance.GetType()
	r.muDag.Lock()
	defer r.muDag.Unlock()
	if r.dag.Lookup(t) != nil {
		return nil, nil, fmt.Errorf("service '%s' is already registered", t)
	}
	if defaultOpts != nil && !defaultOpts.ValidConfigOptsFor(t) {
		return nil, nil, fmt.Errorf("invalid default config options type for %s", t)
	}
	for _, dt := range deps {
		node := r.dag.Lookup(dt)
		if node == nil {
			return nil, nil, fmt.Errorf("service '%s' depends on not-registered service '%s'", t, dt)
		}
	}
	r.dag.AddVertex(t, instance, defaultOpts, autoStart)
	for _, dt := range deps {
		r.dag.AddEdge(dt, t)
	}
	return service{instance, r}, instance, nil
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

// StartAll starts all the services with autoStart flag, unless one of the
// dependencies doesn't autoStart.
func (r *Registry) StartAll() {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	for _, n := range r.dag.Flatten(true) {
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
	if !n.started {
		if n.opts == nil || n.opts.ValidConfigOptsFor(n.t) {
			n.instance.Reconfigure(publisher{n.t, r}, n.opts)
			n.instance.Start()
			n.started = true
		}
	}
	return n.started
}

func (r *Registry) StopAll() {
	r.muDag.RLock()
	defer r.muDag.RUnlock()
	flatten := r.dag.Flatten(false)
	// Stop in reverse order
	for i := len(flatten) - 1; i >= 0; i-- {
		r.stopNoLock(flatten[i])
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
	}
}

// TODO: enforce timeout
func (r *Registry) reconfigure(t Type, fields ConfigUpdates) error {
	r.muDag.Lock()
	defer r.muDag.Unlock()
	n := r.dag.Lookup(t)
	if n.opts == nil {
		return errors.New("%s service doesn't allow config options", t)
	}
	dest := reflect.Indirect(reflect.ValueOf(n.opts))
	if err := r.update(dest, fields); err != nil {
		return err
	}
	r.startNoLock(n)
	return nil
}

func (r *Registry) update(dest reflect.Value, fields ConfigUpdates) error {
	srcFields := make([]reflect.Value, 0, len(fields))
	destFields := make([]reflect.Value, 0, len(fields))
	for k, v := range fields {
		d := dest
		for _, part := range strings.Split(k, ".") {
			d = d.FieldByName(part)
			if !d.IsValid() {
				return errors.New("invalid field %s", part)
			}
		}
		s := reflect.ValueOf(v)
		if d.Type() != s.Type() {
			return errors.New("type mismatch for %s: expect %s, got %s", k, d.Type(), s.Type())
		}
		srcFields = append(srcFields, s)
		destFields = append(destFields, d)
	}
	for i := 0; i < len(srcFields); i++ {
		destFields[i].Set(srcFields[i])
	}
	return nil
}

func (r *Registry) subscribe(t Type) <-chan Message {
	ch := make(chan Message, 1)
	r.muChannels.Lock()
	r.channels[t] = append(r.channels[t], ch)
	r.muChannels.Unlock()
	return ch
}

func (r *Registry) publish(t Type, msg Message) {
	r.muChannels.RLock()
	channels := r.channels[t]
	r.muChannels.RUnlock()
	for _, ch := range channels {
		select {
		case ch <- msg:
		default:
			fmt.Println("Warning: message discarded")
		}
	}
}

type publisher struct {
	t Type
	r *Registry
}

func (p publisher) Publish(m Message) {
	p.r.publish(p.t, m)
}

// service satisfies the Service interface, it forwards all methods to the
// registry.
type service struct {
	impl Impl
	r    *Registry
}

func (s service) Start() bool {
	return s.r.start(s.impl.GetType())
}

func (s service) Started() bool {
	return s.r.started(s.impl.GetType())
}

func (s service) Stop() {
	s.r.stop(s.impl.GetType())
}

func (s service) Reconfigure(fields ConfigUpdates) error {
	return s.r.reconfigure(s.impl.GetType(), fields)
}

func (s service) Subscribe() <-chan Message {
	return s.r.subscribe(s.impl.GetType())
}

func (s service) GetImpl() Impl {
	return s.impl
}
