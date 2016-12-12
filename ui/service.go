package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type helloFnType func(func(interface{}) error) error

type newMsgFnType func() interface{}

type Service struct {
	Type     string
	In       <-chan interface{}
	Out      chan<- interface{}
	in       chan interface{}
	out      chan interface{}
	stopCh   chan bool
	helloFn  helloFnType
	newMsgFn newMsgFnType
}

var (
	mu               sync.RWMutex
	defaultUIChannel *UIChannel

	services = make(map[string]*Service)
)

func (s *Service) write() {
	// Watch for new messages and send them to the combined output.
	for {
		select {
		case <-s.stopCh:
			log.Trace("Received message on stop channel")
			return
		case msg := <-s.out:
			log.Tracef("Creating new envelope for %v", s.Type)
			b, err := newEnvelope(s.Type, msg)
			if err != nil {
				log.Error(err)
				continue
			}
			defaultUIChannel.Out <- b
		}
	}
}

// Register registers a WebSocket based service with an optional helloFn to
// send initial message to connected clients.
func Register(t string, helloFn helloFnType) (*Service, error) {
	return RegisterWithMsgInitializer(t, helloFn, nil)
}

// RegisterWithMsgInitializer is similar to Register, but with an additional
// newMsgFn to initialize the message type to-be received from WebSocket
// client, instead of letting JSON unmarshaler to guess the type.
func RegisterWithMsgInitializer(t string, helloFn helloFnType, newMsgFn newMsgFnType) (*Service, error) {
	log.Tracef("Registering UI service %s", t)
	mu.Lock()

	if services[t] != nil {
		// Using panic because this would be a developer error rather that
		// something that could happen naturally.
		panic("Service was already registered.")
	}

	s := &Service{
		Type:     t,
		in:       make(chan interface{}, 100),
		out:      make(chan interface{}, 100),
		stopCh:   make(chan bool),
		helloFn:  helloFn,
		newMsgFn: newMsgFn,
	}
	s.In, s.Out = s.in, s.out

	// Sending existent clients the hello message of the new service.
	if helloFn != nil {
		err := helloFn(func(msg interface{}) error {
			b, err := newEnvelope(s.Type, msg)
			if err != nil {
				return err
			}
			log.Tracef("Sending initial message to existent clients")
			defaultUIChannel.Out <- b
			return nil
		})
		if err != nil {
			log.Debugf("Error running Hello function", err)
		}
	}

	// Adding new service to service map.
	services[t] = s
	mu.Unlock()

	log.Tracef("Registered UI service %s", t)
	go s.write()
	return s, nil
}

func Unregister(t string) {
	log.Tracef("Unregistering service: %v", t)
	if services[t] != nil {
		services[t].stopCh <- true
		delete(services, t)
	}
}

// To facilitate test
func unregisterAll() {
	for t, _ := range services {
		Unregister(t)
	}
}

func startUIChannel() {
	// Establish a channel to the UI for sending and receiving updates
	defaultUIChannel = NewChannel("/data", func(write func([]byte) error) error {
		// Sending hello messages.
		mu.RLock()
		for _, s := range services {
			// Delegating task...
			if s.helloFn != nil {
				writer := func(msg interface{}) error {
					b, err := newEnvelope(s.Type, msg)
					if err != nil {
						return err
					}
					return write(b)
				}

				if err := s.helloFn(writer); err != nil {
					log.Errorf("Error writing to socket: %q", err)
				}
			}
		}
		mu.RUnlock()
		return nil
	})

	go readLoop(defaultUIChannel.In)

	log.Debugf("Accepting websocket connections at: %s", defaultUIChannel.URL)
}

func readLoop(in <-chan []byte) {
	for b := range in {
		// Determining message type.
		var envType EnvelopeType
		err := json.Unmarshal(b, &envType)

		if err != nil {
			log.Errorf("Unable to parse JSON update from browser: %q", err)
			continue
		}

		// Delegating response to the service that registered with the given type.
		service := services[envType.Type]
		if service == nil {
			log.Errorf("Message type %v belongs to an unknown service.", envType.Type)
			continue
		}

		env := &Envelope{}
		if service.newMsgFn != nil {
			env.Message = service.newMsgFn()
		}
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.UseNumber()
		err = d.Decode(env)
		if err != nil {
			log.Errorf("Unable to unmarshal message of type %v: %v", envType.Type, err)
			continue
		}
		log.Tracef("Forwarding message: %v", env)
		// Pass this message and continue reading another one.
		service.in <- env.Message
	}
}

func newEnvelope(t string, msg interface{}) ([]byte, error) {
	b, err := json.Marshal(&Envelope{
		EnvelopeType: EnvelopeType{t},
		Message:      msg,
	})
	if err != nil {
		return nil, fmt.Errorf("Unable to marshal message of type %v: %v", t, msg)
	}
	return b, nil
}
