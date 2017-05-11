package geolookup

import (
	"testing"
	"time"

	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/fronted"
)

func TestFronted(t *testing.T) {
	s, i := service.NewRegistry().MustRegister(New, nil, true, nil)
	ch := s.Subscribe()
	s.Start()
	instance := i.(*GeoLookup)
	instance.Refresh()
	fronted.ConfigureForTest(t)
	country := instance.GetCountry(15 * time.Second)
	ip := instance.GetIP(0)
	if len(country) != 2 {
		t.Fatalf("Bad country '%v' for ip %v", country, ip)
	}
	if len(ip) < 7 {
		t.Fatalf("Bad IP %s", ip)
	}
	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Error("should update watcher")
	}
}
