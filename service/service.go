// Package service provides mechanism and interfaces to declare, register,
// lookup, and manage the lifecycle of a group of services, i.e., long-running
// tasks.
//
package service

import "github.com/getlantern/golog"

// ID uniquely identify a service. Typically, each service defines a package
// level constant ServiceID with an unique string.
type ID string

// Service is the minimum interface to implemetent a service.
type Service interface {
	// GetID returns the id of the service. It should uniquely identify a server.
	GetID() ID
	// Start actually starts the service. The Registry calls it only once until
	// it's stopped. To avoid deadlock, Start should return as soon as possible
	// and should not call methods in this package.
	Start()
	// Stop actually stops the service. The Registry calls it only if the
	// service was started. To avoid deadlock, Stop should return as soon as
	// possible and should not call methods in this package.
	Stop()
}

// Configurable is an interface that service can choose to implement to
// configure itself dynamically.
type Configurable interface {
	// Configure configures the service with current effective config
	// options. Registry only calls this when the ConfigOpts are Complete().
	// Serviceement carefully To avoid data races. The implementation can choose
	// to restart the service internally when some configuration changes, but
	// it doesn't affect the service status from the outside.
	Configure(opts ConfigOpts)
}

// ConfigOpts represents all of the config options required to start a service.
type ConfigOpts interface {
	// For returns the service id to which the ConfigOpts apply.
	For() ID
	// Complete checks if the ConfigOpts is complete to start the service. If
	// not, return the reason.
	Complete() string
}

// WillPublish is an interface the service can optionally implement to publish
// any message.
type WillPublish interface {
	// SetPublisher is for services to store the publisher for further reference.
	SetPublisher(p Publisher)
}

// Publisher is what a service can optionally use to publish a message.
type Publisher interface {
	// Publish publishes any message to all of the subscribers.
	Publish(msg interface{})
}

var (
	singleton *Registry
	log       = golog.LoggerFor("flashlight.service")
)

func init() {
	singleton = NewRegistry()
}

// MustRegister calls MustRegister of the singleton registry
func MustRegister(instance Service, defaultOpts ConfigOpts) {
	singleton.MustRegister(instance, defaultOpts)
}

// Register calls Register of the singleton registry
func Register(instance Service, defaultOpts ConfigOpts) error {
	return singleton.Register(instance, defaultOpts)
}

// MustLookup calls MustLookup of the singleton registry
func MustLookup(id ID) Service {
	return singleton.MustLookup(id)
}

// Lookup calls Lookup of the singleton registry
func Lookup(id ID) Service {
	return singleton.Lookup(id)
}

// MustConfigure calls MustConfigure of the singleton registry
func MustConfigure(id ID, op func(opts ConfigOpts)) {
	singleton.MustConfigure(id, op)
}

// Configure calls Configure of the singleton registry
func Configure(id ID, op func(opts ConfigOpts)) error {
	return singleton.Configure(id, op)
}

// MustSubCh calls MustSubCh of the singleton registry.
func MustSubCh(id ID) <-chan interface{} {
	return singleton.MustSubCh(id)
}

// SubCh calls SubCh of the singleton registry.
func SubCh(id ID) (<-chan interface{}, error) {
	return singleton.SubCh(id)
}

// MustSub calls MustSub of the singleton registry.
func MustSub(id ID, cb func(m interface{})) {
	singleton.MustSub(id, cb)
}

// Sub calls Sub of the singleton registry.
func Sub(id ID, cb func(m interface{})) error {
	return singleton.Sub(id, cb)
}

// Start calls Start of the singleton registry.
func Start(id ID) bool {
	return singleton.Start(id)
}

// Stop calls Stop of the singleton registry.
func Stop(id ID) {
	singleton.Stop(id)
}

// StartAll starts all services registered to the singleton registry.
func StartAll() {
	singleton.StartAll()
}

// CloseAll stops all started services registered to the singleton registry and
// close all subscribed channels.
func CloseAll() {
	singleton.CloseAll()
}

type publisher struct {
	id ID
	r  *Registry
}

func (p publisher) Publish(msg interface{}) {
	p.r.publish(p.id, msg)
}
