package desktop

import (
	"sync"

	"github.com/getlantern/flashlight/ws"

	log "github.com/sirupsen/logrus"
)

type UserSignal struct {
	service *ws.Service
	once    sync.Once
}

var userSignal UserSignal

func setupUserSignal(channel ws.UIChannel, connect func(), disconnect func()) {
	userSignal.once.Do(func() {
		err := userSignal.start(channel)
		if err != nil {
			log.Errorf("Unable to register signal service: %q", err)
			return
		}
		go userSignal.read(connect, disconnect)
	})
}

// start the settings service that synchronizes Lantern's configuration with every UI client
func (s *UserSignal) start(channel ws.UIChannel) error {
	var err error
	helloFn := func(write func(interface{})) {
		write("connected")
	}
	s.service, err = channel.Register("signal", helloFn)
	return err
}

func (s *UserSignal) read(connect func(), disconnect func()) {
	for message := range s.service.In {
		log.Infof("Read userSignal %v", message)
		switch message {
		case "disconnect":
			disconnect()
		case "connect":
			connect()
		default:
			continue
		}
	}
}
