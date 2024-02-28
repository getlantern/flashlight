package bandit

import (
	"strings"
)

type location struct {
	city        string
	country     string
	countryCode string
}

var (
	// NOTE: it should contain all values in SMEMBERS cloudmasters
	dcLocs = map[string]*location{
		"doams3": {
			"Amsterdam",
			"Netherlands",
			"NL",
		},
		"doblr1": {
			"Bangalore",
			"India",
			"IN",
		},
		"dofra1": {
			"Frankfurt",
			"Germany",
			"DE",
		},
		"dolon1": {
			"London",
			"United Kingdom",
			"GB",
		},
		"dosgp1": {
			"Singapore",
			"Singapore",
			"SG",
		},
		"dosfo1": {
			"San Francisco",
			"United States",
			"US",
		},
		"donyc3": {
			"New York",
			"United States",
			"US",
		},
		"lisgp1": {
			"Singapore",
			"Singapore",
			"SG",
		},
		"litok1": {
			"Tokyo",
			"Japan",
			"JP",
		},
		"litok2": {
			"Tokyo",
			"Japan",
			"JP",
		},
		"xfkrkt": {
			"Seoul",
			"Korea",
			"KR",
		},
		"ggtwnb": {
			"Changhua",
			"Taiwan",
			"TW",
		},
		"vllhr1": {
			"London",
			"United Kingdom",
			"GB",
		},
		"vllos1": {
			"Los Angeles",
			"United States",
			"US",
		},
		"vlsgp1": {
			"Singapore",
			"Singapore",
			"SG",
		},
		"vltok1": {
			"Tokyo",
			"Japan",
			"JP",
		},
		"vltok9": {
			"Tokyo",
			"Japan",
			"JP",
		},
		"anhk1b": {
			"Hong Kong",
			"China",
			"HK",
		},
		"sbtk1a": {
			"Tokyo",
			"Japan",
			"JP",
		},
		"sbtk9a": {
			"Tokyo",
			"Japan",
			"JP",
		},
		"sbhk1b": {
			"Hong Kong",
			"China",
			"HK",
		},
		"cdsgz1": {
			"Hong Kong",
			"China",
			"HK",
		},
		"enhttp": {
			"New York",
			"United States",
			"US",
		},
	}
)

func proxyLoc(proxy Dialer) (countryCode string, country string, city string) {
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
	log.Debugf("Couldn't find location for %s", name)
	return "N/A", "N/A", "N/A"
}
