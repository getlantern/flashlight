package signal

import (
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.desktop.signal")

	ServiceID service.ID = "flashlight.desktop.signal"
)

type userSignal struct {
	service *ws.Service
	chStop  chan struct{}
	p       service.Publisher
}

func New() service.Service {
	return &userSignal{}
}

func (s *userSignal) GetID() service.ID {
	return ServiceID
}

func (s *userSignal) SetPublisher(p service.Publisher) {
	s.p = p
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
				s.p.Publish(false)
			case "connect":
				s.p.Publish(true)
			default:
				continue
			}
			s.service.Out <- message
		case <-s.chStop:
			return
		}
	}
}
