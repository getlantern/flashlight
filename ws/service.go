package ws

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// envelopeType is the type of the message envelope.
type envelopeType struct {
	Type string `json:"type,inline"`
}

// envelope is a struct that wraps messages and associates them with a type.
type envelope struct {
	envelopeType
	Message interface{} `json:"message"`
}

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
	defaultUIChannel *UIChannel
	muServices       sync.RWMutex
	services         = make(map[string]*Service)
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
	muServices.Lock()
	defer muServices.Unlock()

	if services[t] != nil {
		// Using panic because this would be a developer error rather that
		// something that could happen naturally.
		panic("Service was already registered.")
	}

	s := &Service{
		Type:     t,
		in:       make(chan interface{}, 100),
		out:      make(chan interface{}, 100),
		stopCh:   make(chan bool, 1), // buffered to avoid blocking `Unregister()`
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

	log.Tracef("Registered UI service %s", t)
	go s.write()
	return s, nil
}

func Unregister(t string) {
	log.Tracef("Unregistering service: %v", t)
	muServices.Lock()
	defer muServices.Unlock()
	if services[t] != nil {
		services[t].stopCh <- true
		delete(services, t)
	}
}

// StartUIChannel establishes a channel to the UI for sending and receiving
// updates
func StartUIChannel() *UIChannel {
	defaultUIChannel = NewChannel(func(write func([]byte) error) error {
		// Sending hello messages.
		muServices.RLock()
		defer muServices.RUnlock()
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
		return nil
	})

	go readLoop(defaultUIChannel.In)

	log.Debugf("Accepting WebSocket connections")
	return defaultUIChannel
}

func readLoop(in <-chan []byte) {
	for b := range in {
		// Determining message type.
		var envType envelopeType
		err := json.Unmarshal(b, &envType)

		if err != nil {
			log.Errorf("Unable to parse JSON update from browser: %q", err)
			continue
		}

		// Delegating response to the service that registered with the given type.
		muServices.RLock()
		service := services[envType.Type]
		muServices.RUnlock()
		if service == nil {
			log.Errorf("Message type %v belongs to an unknown service.", envType.Type)
			continue
		}

		env := &envelope{}
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
	b, err := json.Marshal(&envelope{
		envelopeType: envelopeType{t},
		Message:      msg,
	})
	if err != nil {
		return nil, fmt.Errorf("Unable to marshal message of type %v: %v", t, msg)
	}
	return b, nil
}
