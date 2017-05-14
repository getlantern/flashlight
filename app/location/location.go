package location

import (
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
)

var ServiceType service.Type = "ws.location"
var wsServiceType = "location"

func New() service.Impl {
	return &locationService{}
}

type ConfigOpts struct {
	Code string `json:"code"`
}

func (d ConfigOpts) For() service.Type {
	return ServiceType
}

func (d ConfigOpts) Complete() bool {
	return d.Code != ""
}

type locationService struct {
	loc *ConfigOpts
}

func (s *locationService) GetType() service.Type {
	return ServiceType
}

func (s *locationService) Reconfigure(_ service.Publisher, loc service.ConfigOpts) {
	s.loc = loc.(*ConfigOpts)
}

func (s *locationService) Start() {
	helloFn := func(write func(interface{})) {
		write(s.loc)
	}
	// ws.Register always succeed
	ws.Register(wsServiceType, helloFn)
}

func (s *locationService) Stop() {
	ws.Unregister(wsServiceType)
}
