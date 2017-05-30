package signal

import (
	"sync"

	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/app/sysproxy"
)

type UserSignal struct {
	service *ws.Service
	once    sync.Once
}

var (
	log = golog.LoggerFor("flashlight.desktop.signal")

	userSignal UserSignal
)

func Start() {
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
			service.Start(sysproxy.ServiceID)
		case "connect":
			service.Stop(sysproxy.ServiceID)
		default:
			continue
		}
		s.service.Out <- message
	}
}
