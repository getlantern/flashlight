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
// configure the service itself based on the message.
type Deps map[Type]func(msg interface{}, self Service)

// ConfigOpts represents all of the config options required to start a service.
type ConfigOpts interface {
	// For returns the service type to which the ConfigOpts apply
	For() Type
	// Complete checks if the ConfigOpts is complete to start the service. If
	// not, return the reason.
	Complete() string
}

// Service is the reference to a service, which is return by
// Registry.Register() or Registry.Lookup().
type Service interface {
	// Start checks if the effective config options is valid, configure the
	// service with it and starts the service.  It returns if the service is
	// currently started.  Starting an already started service is a noop and
	// returns true.
	Start() bool
	// Started returns true if service is started, false otherwise.
	Started() bool
	// Stop stops a started service. Stopping an unstarted service is a noop.
	Stop()
	// Configure updates part of the effective config options. If the option
	// is valid, it calls Configure() of the Impl and start the service if
	// not already started.
	Configure(func(opts ConfigOpts)) error
	// MustConfigure is the same as Configure, but panics if error happens.
	MustConfigure(func(opts ConfigOpts))
	// GetImpl gets the implementation of the service. Caller usually casts it
	// to a concrete type to call its specific methods. Be aware that one
	// should always Start(), Stop(), or Configure() via the Service, instead
	// of the methods of the Impl.
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
	// GetType returns the type of the service.
	GetType() Type
	// Start actually starts the service. The Registry calls it only once until
	// it's stopped.
	Start()
	// Stop actually stops the service. The Registry calls it only if the
	// service was started.
	Stop()
}

// Configurable is an interface that service can choose to implement to
// configure itself in runtime.
type Configurable interface {
	// Configure configures the service with current effective config
	// options. Registry only calls this when the ConfigOpts are Complete().
	// Implement carefully To avoid data races. The implementation can choose
	// to restart the service internally when some configuration changes, but
	// it doesn't affect the service status from the outside.
	Configure(opts ConfigOpts)
}

type WillPublish interface {
	SetPublisher(p Publisher)
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
	instance Impl,
	defaultOpts ConfigOpts,
	deps Deps) (Service, Impl) {
	return singleton.MustRegister(instance, defaultOpts, deps)
}

// Register registers the service to the singleton registry
func Register(
	instance Impl,
	defaultOpts ConfigOpts,
	deps Deps) (Service, Impl, error) {
	return singleton.Register(instance, defaultOpts, deps)
}

// MustLookup looks up a service from the singleton registry, or panics.
func MustLookup(t Type) (Service, Impl) {
	return singleton.MustLookup(t)
}

// Lookup looks up a service from the singleton registry, or nil.
func Lookup(t Type) (Service, Impl) {
	return singleton.Lookup(t)
}

// MustConfigure configures a service from the singleton registry, or panics.
func MustConfigure(t Type, op func(opts ConfigOpts)) {
	singleton.MustConfigure(t, op)
}

// Configure configures  a service from the singleton registry.
func Configure(t Type, op func(opts ConfigOpts)) error {
	return singleton.Configure(t, op)
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

func (s service) MustConfigure(op func(ConfigOpts)) {
	if err := s.Configure(op); err != nil {
		panic(err)
	}
}

func (s service) Configure(op func(ConfigOpts)) error {
	return s.r.Configure(s.impl.GetType(), op)
}

func (s service) GetImpl() Impl {
	return s.impl
}
