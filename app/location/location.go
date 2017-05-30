package location

import (
	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/flashlight/ws"
)

var ServiceID service.ID = "ws.location"
var wsServiceID = "location"

func New() service.Service {
	return &locationService{}
}

type ConfigOpts struct {
	Code string `json:"code"`
}

func (d ConfigOpts) For() service.ID {
	return ServiceID
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

func (s *locationService) GetID() service.ID {
	return ServiceID
}

func (s *locationService) Configure(loc service.ConfigOpts) {
	s.loc = loc.(*ConfigOpts)
}

func (s *locationService) Start() {
	helloFn := func(write func(interface{})) {
		write(s.loc)
	}
	// ws.Register always succeed
	ws.Register(wsServiceID, helloFn)
}

func (s *locationService) Stop() {
	ws.Unregister(wsServiceID)
}
