package geolookup

import (
	"testing"
	"time"

	"github.com/getlantern/fronted"
)

func TestFronted(t *testing.T) {
	fronted.ConfigureForTest(t)
	ch := OnRefresh()
	Refresh()
	country := GetCountry(15 * time.Second)
	ip := GetIP(5 * time.Second)
	if len(country) != 2 {
		t.Fatalf("Bad country '%v' for ip %v", country, ip)
	}
	if len(ip) < 7 {
		t.Fatalf("Bad IP %s", ip)
	}

	if !<-ch {
		t.Error("should update watcher")
	}
}
