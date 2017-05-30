package geolookup

import (
	"testing"
	"time"

	"github.com/getlantern/flashlight/service"
	"github.com/getlantern/fronted"
)

func TestFronted(t *testing.T) {
	r := service.NewRegistry()
	i := New()
	r.MustRegister(i, nil)
	ch := r.MustSubCh(ServiceID)
	r.Start(ServiceID)
	instance := i.(*GeoLookup)
	fronted.ConfigureForTest(t)
	instance.Refresh()
	select {
	case m := <-ch:
		info := m.(*GeoInfo)
		country, ip := info.GetCountry(), info.GetIP()
		if len(country) != 2 {
			t.Fatalf("Bad country '%v' for ip %v", country, ip)
		}
		if len(ip) < 7 {
			t.Fatalf("Bad IP %s", ip)
		}
	case <-time.After(1 * time.Minute):
		t.Error("should update watcher")
	}
}
