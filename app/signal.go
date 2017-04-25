package app

import (
	"sync"

	"github.com/getlantern/flashlight/ws"
)

type UserSignal struct {
	service *ws.Service
	once    sync.Once
}

var userSignal UserSignal

func setupUserSignal() {
	userSignal.once.Do(func() {
		err := userSignal.start()
		if err != nil {
			log.Errorf("Unable to register signal service: %q", err)
			return
		}
		go userSignal.read()
	})
}

// start the settings service that synchronizes Lantern's configuration with every UI client
func (s *UserSignal) start() error {
	var err error
	helloFn := func(write func(interface{})) {
		write("connected")
	}
	s.service, err = ws.Register("signal", helloFn)
	return err
}

func (s *UserSignal) read() {
	for message := range s.service.In {
		log.Debugf("Read userSignal %v", message)
		switch message {
		case "disconnect":
			sysproxyOff()
		case "connect":
			sysproxyOn()
		default:
			continue
		}
		s.service.Out <- message
	}
}
