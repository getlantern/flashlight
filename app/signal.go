package app

import (
	"sync"

	"github.com/getlantern/flashlight/ui"
)

type UserSignal struct {
	service *ui.Service
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
	helloFn := func(write func(interface{}) error) error {
		return write("connected")
	}
	s.service, err = ui.Register("signal", helloFn)
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
