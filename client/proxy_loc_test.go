package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProxyLoc(t *testing.T) {
	valid_proxies := map[string]string{
		"fp-donyc3-20161214-001":              "New York",
		"fp-anhk1b-20161214-001":              "Hong Kong",
		"fp-https-donyc3-20161214-001":        "New York",
		"fp-obfs4-donyc3-20161214-001":        "New York",
		"fp-obfs4-donyc3staging-20161214-001": "New York",
	}
	for proxy, city := range valid_proxies {
		loc := proxyLoc(proxy)
		if assert.NotNil(t, loc) {
			assert.Equal(t, loc.city, city)
		}
	}

	invalid_proxies := []string{
		"fp-anhklb-20161214-001",
	}
	for _, proxy := range invalid_proxies {
		assert.Nil(t, proxyLoc(proxy))
	}
}
