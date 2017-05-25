// Package service provides mechanism and interfaces to declare, register,
// lookup, and manage the lifecycle of a group of services, i.e., long-running
// tasks.
//
package service

import "github.com/getlantern/golog"

// Type uniquely identify a service. Typically, each service defines a package
// level constant ServiceType with an unique string.
type Type string

// ConfigOpts represents all of the config options required to start a service.
type ConfigOpts interface {
	// For returns the service type to which the ConfigOpts apply
	For() Type
	// Complete checks if the ConfigOpts is complete to start the service. If
	// not, return the reason.
	Complete() string
}

// Publisher is an interface for the service impletation to publish a message,
// when required.
type Publisher interface {
	// Publish publishes any message to all of the subscribers.
	Publish(msg interface{})
}

// Service actually implemetents the service.
type Service interface {
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
	// Serviceement carefully To avoid data races. The implementation can choose
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
func MustRegister(instance Service, defaultOpts ConfigOpts) {
	singleton.MustRegister(instance, defaultOpts)
}

// Register registers the service to the singleton registry
func Register(instance Service, defaultOpts ConfigOpts) error {
	return singleton.Register(instance, defaultOpts)
}

// MustLookup looks up a service from the singleton registry, or panics.
func MustLookup(t Type) Service {
	return singleton.MustLookup(t)
}

// Lookup looks up a service from the singleton registry, or nil.
func Lookup(t Type) Service {
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

// SubCh calls SubCh of the singleton registry.
func SubCh(t Type) <-chan interface{} {
	return singleton.SubCh(t)
}

// Sub calls Sub of the singleton registry.
func Sub(t Type, cb func(m interface{})) {
	singleton.Sub(t, cb)
}

func Start(t Type) bool {
	return singleton.Start(t)
}

func Stop(t Type) {
	singleton.Stop(t)
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
