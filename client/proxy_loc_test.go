package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type locationDialer struct {
	testDialer
	countryCode, country, city string
}

func (d *locationDialer) Location() (string, string, string) {
	return d.countryCode, d.country, d.city
}

func newLocationDialer(name, countryCode string) *locationDialer {
	return &locationDialer{testDialer: testDialer{name: name}, countryCode: countryCode}
}

func TestProxyLoc(t *testing.T) {
	embed_in_config := map[string]string{
		"fp-usany1-20161214-001":    "US",
		"fp-hongkong1-20161214-001": "HK",
	}
	for proxy, cc := range embed_in_config {
		code, _, _ := proxyLoc(newLocationDialer(proxy, cc))
		assert.Equal(t, code, cc, "should use location info embeded in proxy config")
	}

	with_hardcoded_location := map[string]string{
		"fp-donyc3-20161214-001":              "New York",
		"fp-anhk1b-20161214-001":              "Hong Kong",
		"fp-https-donyc3-20161214-001":        "New York",
		"fp-obfs4-donyc3-20161214-001":        "New York",
		"fp-obfs4-donyc3staging-20161214-001": "New York",
	}
	for proxy, c := range with_hardcoded_location {
		code, _, city := proxyLoc(newLocationDialer(proxy, ""))
		if assert.NotEmpty(t, code) {
			assert.Equal(t, city, c, "should use hardcoded data if no location in config")
		}
	}

	invalid_proxies := []string{
		"fp-anhklb-20161214-001",
	}
	for _, proxy := range invalid_proxies {
		code, country, city := proxyLoc(newLocationDialer(proxy, ""))
		assert.Equal(t, "N/A", code)
		assert.Equal(t, "N/A", country)
		assert.Equal(t, "N/A", city)
	}
}
