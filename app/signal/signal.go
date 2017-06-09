package signal

import (
	"reflect"

	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.desktop.signal")
)

type userSignal struct {
	service *ws.Service
	chStop  chan struct{}
	service.PubSub
}

func New() service.PubSubService {
	return &userSignal{}
}

func (s *userSignal) GetID() service.ID {
	return reflect.TypeOf(s)
}

func (s *userSignal) Start() {
	s.chStop = make(chan struct{})
	var err error
	s.service, err = ws.Register("signal", func(write func(interface{})) {
		write("connected")
	})
	if err != nil {
		log.Error(err)
		return
	}
	go s.read()
}

func (s *userSignal) Stop() {
	s.chStop <- struct{}{}
}

func (s *userSignal) read() {
	for {
		select {
		case message := <-s.service.In:
			if message == nil { // when channel is closed
				return
			}
			log.Debugf("Read userSignal %v", message)
			switch message {
			case "disconnect":
				s.Pub(false)
			case "connect":
				s.Pub(true)
			default:
				continue
			}
			s.service.Out <- message
		case <-s.chStop:
			return
		}
	}
}
