package client

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
	}
)

func proxyLoc(name string) *location {
	for k, v := range dcLocs {
		if strings.Contains(name, k) {
			return v
		}
	}
	return nil
}
