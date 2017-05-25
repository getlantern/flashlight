package location

import (
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
)

var ServiceType service.Type = "ws.location"
var wsServiceType = "location"

func New() service.Service {
	return &locationService{}
}

type ConfigOpts struct {
	Code string `json:"code"`
}

func (d ConfigOpts) For() service.Type {
	return ServiceType
}

func (d ConfigOpts) Complete() string {
	if d.Code == "" {
		return "missing Code"
	}
	return ""
}

type locationService struct {
	loc *ConfigOpts
}

func (s *locationService) GetType() service.Type {
	return ServiceType
}

func (s *locationService) Configure(loc service.ConfigOpts) {
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
