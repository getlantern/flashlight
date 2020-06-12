package geolookup

import (
	"testing"
	"time"

	"github.com/getlantern/flashlight/frontedfl"
)

func TestFronted(t *testing.T) {
	frontedfl.ConfigureHostAliasesForTest(t, map[string]string{
		"geo.getiantem.org": "d3u5fqukq7qrhd.cloudfront.net",
	})
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
