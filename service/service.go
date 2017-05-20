// Package service provides mechanism and interfaces to declare, register,
// lookup, and manage the lifecycle of a group of services, i.e., long-running
// tasks.
//
package service

import "github.com/getlantern/golog"

// Type uniquely identify a service. Typically, each service defines a package
// level constant ServiceType with an unique string.
type Type string

// Deps represents the services on which one service depends, and optional
// handler to process message from the depended service. Typically, the handler
// reconfigure the service itself based on the message.
type Deps map[Type]func(msg interface{}, self Service)

// ConfigOpts represents all of the config options required to start a service.
type ConfigOpts interface {
	// For returns the service type to which the ConfigOpts apply
	For() Type
	// Complete checks if the ConfigOpts is complete to start the service. If
	// not, return the reason.
	Complete() string
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
	Reconfigure(func(opts ConfigOpts)) error
	// MustReconfigure is the same as Reconfigure, but panics if error happens.
	MustReconfigure(func(opts ConfigOpts))
	// Subscribe returns a channel to receive any message the service published.
	// Messages are discarded if no one is listening on the channel.
	Subscribe() <-chan interface{}
	// GetImpl gets the implementation of the service. Caller usually casts it
	// to a concrete type to call its specific methods. Be aware that one
	// should always Start(), Stop(), or Reconfigure() via the Service, instead
	// of the Impl.
	GetImpl() Impl
}

// Publisher is an interface for the service impletation to publish a message,
// when required.
type Publisher interface {
	// Publish publishes any message to all of the subscribers.
	Publish(msg interface{})
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

var (
	singleton *Registry
	log       = golog.LoggerFor("flashlight.service")
)

func init() {
	singleton = NewRegistry()
}

// MustRegister registers the service to the singleton registry, or panics.
func MustRegister(
	instantiator func() Impl,
	defaultOpts ConfigOpts,
	autoStart bool,
	deps Deps) (Service, Impl) {
	return singleton.MustRegister(instantiator, defaultOpts, autoStart, deps)
}

// Register registers the service to the singleton registry
func Register(
	instantiator func() Impl,
	defaultOpts ConfigOpts,
	autoStart bool,
	deps Deps) (Service, Impl, error) {
	return singleton.Register(instantiator, defaultOpts, autoStart, deps)
}

// MustLookup looks up a service from the singleton registry, or panics.
func MustLookup(t Type) (Service, Impl) {
	return singleton.MustLookup(t)
}

// Lookup looks up a service from the singleton registry, or nil.
func Lookup(t Type) (Service, Impl) {
	return singleton.Lookup(t)
}

// MustReconfigure configures a service from the singleton registry, or panics.
func MustReconfigure(t Type, op func(opts ConfigOpts)) {
	singleton.MustReconfigure(t, op)
}

// Reconfigure configures  a service from the singleton registry.
func Reconfigure(t Type, op func(opts ConfigOpts)) error {
	return singleton.Reconfigure(t, op)
}

// Subscribe subscribes message of a service from the singleton registry.
func Subscribe(t Type) <-chan interface{} {
	return singleton.Subscribe(t)
}

// StartAll starts all services registered to the singleton registry.
func StartAll() {
	singleton.StartAll()
}

// StopAll stops all started services registered to the singleton registry.
func StopAll() {
	singleton.StopAll()
}

type publisher struct {
	t Type
	r *Registry
}

func (p publisher) Publish(msg interface{}) {
	p.r.publish(p.t, msg)
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

func (s service) MustReconfigure(op func(ConfigOpts)) {
	if err := s.Reconfigure(op); err != nil {
		panic(err)
	}
}

func (s service) Reconfigure(op func(ConfigOpts)) error {
	return s.r.Reconfigure(s.impl.GetType(), op)
}

func (s service) Subscribe() <-chan interface{} {
	return s.r.Subscribe(s.impl.GetType())
}

func (s service) GetImpl() Impl {
	return s.impl
}
