package service

import "fmt"

type Type string

type ConfigOpts interface {
	ValidConfigOptsFor(t Type) bool
}

type Message interface {
	ValidMessageFrom(t Type) bool
}

type Service interface {
	Start()
	Stop()
	Reconfigure(opts ConfigOpts)
	Subscribe() chan<- Message
}

type Publisher interface {
	Publish(Type, Message)
}

type Impl interface {
	GetType() Type
	Start()
	Stop()
	Reconfigure(p Publisher, opts ConfigOpts)
}

type Registry struct {
}

func (r *Registry) Register(instantiator func() Impl, dependencies []Type) {
}

func (r *Registry) Lookup(Type) Service {
	return nil
}

func (r *Registry) MustLookup(t Type) Service {
	if s := r.Lookup(t); s != nil {
		return s
	}
	panic(fmt.Sprintf("MustLookup service type '%s'", string(t)))
}

func (r *Registry) Publish(Type, Message) {
}
