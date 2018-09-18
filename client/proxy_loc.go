package client

import (
	"strings"

	"github.com/getlantern/flashlight/balancer"
)

type location struct {
	city        string
	country     string
	countryCode string
}

var (
	// NOTE: it should contain all values in SMEMBERS cloudmasters
	dcLocs = map[string]*location{
		"doams3": &location{
			"Amsterdam",
			"Netherlands",
			"NL",
		},
		"doblr1": &location{
			"Bangalore",
			"India",
			"IN",
		},
		"dofra1": &location{
			"Frankfurt",
			"Germany",
			"DE",
		},
		"dolon1": &location{
			"London",
			"United Kingdom",
			"GB",
		},
		"dosgp1": &location{
			"Singapore",
			"Singapore",
			"SG",
		},
		"dosfo1": &location{
			"San Francisco",
			"United States",
			"US",
		},
		"donyc3": &location{
			"New York",
			"United States",
			"US",
		},
		"lisgp1": &location{
			"Singapore",
			"Singapore",
			"SG",
		},
		"litok1": &location{
			"Tokyo",
			"Japan",
			"JP",
		},
		"litok2": &location{
			"Tokyo",
			"Japan",
			"JP",
		},
		"xfkrkt": &location{
			"Seoul",
			"Korea",
			"KR",
		},
		"ggtwnb": &location{
			"Changhua",
			"Taiwan",
			"TW",
		},
		"vllhr1": &location{
			"London",
			"United Kingdom",
			"GB",
		},
		"vllos1": &location{
			"Los Angeles",
			"United States",
			"US",
		},
		"vlsgp1": &location{
			"Singapore",
			"Singapore",
			"SG",
		},
		"vltok1": &location{
			"Tokyo",
			"Japan",
			"JP",
		},
		"vltok9": &location{
			"Tokyo",
			"Japan",
			"JP",
		},
		"anhk1b": &location{
			"Hong Kong",
			"China",
			"HK",
		},
		"sbtk1a": &location{
			"Tokyo",
			"Japan",
			"JP",
		},
		"sbtk9a": &location{
			"Tokyo",
			"Japan",
			"JP",
		},
		"sbhk1b": &location{
			"Hong Kong",
			"China",
			"HK",
		},
		"cdsgz1": &location{
			"Hong Kong",
			"China",
			"HK",
		},
		"enhttp": &location{
			"New York",
			"United States",
			"US",
		},
	}
)

func proxyLoc(proxy balancer.Dialer) (countryCode string, country string, city string) {
	countryCode, country, city = proxy.Location()
	if countryCode != "" {
		return
	}
	// proxies launched before June 2018 don't have location info in
	// the config, fallback to hardcoded data.
	name := proxy.Name()
	for k, v := range dcLocs {
		if strings.Contains(name, k) {
			return v.countryCode, v.country, v.city
		}
	}
	log.Infof("Couldn't find location for %s", name)
	return "N/A", "N/A", "N/A"
}
