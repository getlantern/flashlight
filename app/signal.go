package app

import (
	"sync"

	"github.com/getlantern/flashlight/ws"
	"github.com/getlantern/golog"
)

type UserSignal struct {
	service *ws.Service
	once    sync.Once
	log     golog.Logger
	sp      *systemproxy
}

var userSignal UserSignal

func setupUserSignal(sp *systemproxy) {
	userSignal.log = golog.LoggerFor("app.usersignal")
	userSignal.sp = sp
	userSignal.once.Do(func() {
		err := userSignal.start()
		if err != nil {
			userSignal.log.Errorf("Unable to register signal service: %q", err)
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
		s.log.Debugf("Read userSignal %v", message)
		switch message {
		case "disconnect":
			s.sp.sysproxyOff()
		case "connect":
			s.sp.sysproxyOn()
		default:
			continue
		}
		s.service.Out <- message
	}
}
