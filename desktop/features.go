package desktop

import (
	"sync"

	"github.com/getlantern/flashlight/ws"
)

type featuresService struct {
	service         *ws.Service
	featuresEnabled map[string]bool
	mx              sync.Mutex
}

func NewFeaturesService() *featuresService {
	return &featuresService{
		featuresEnabled: make(map[string]bool),
	}
}

func (s *featuresService) Update(features map[string]bool) {
	s.mx.Lock()
	s.featuresEnabled = features
	s.mx.Unlock()
	select {
	case s.service.Out <- features:
		// ok
	default:
		// don't block if no-one is listening
	}
}

func (s *featuresService) StartService(channel ws.UIChannel) (err error) {
	helloFn := func(write func(interface{})) {
		log.Debugf("Sending features enabled to new client")
		s.mx.Lock()
		featuresEnabled := s.featuresEnabled
		s.mx.Unlock()
		write(featuresEnabled)
	}

	s.service, err = channel.Register("features", helloFn)
	return
}
