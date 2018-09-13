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
	for proxy, c := range valid_proxies {
		code, _, city := proxyLoc(proxy)
		if assert.NotEmpty(t, code) {
			assert.Equal(t, city, c)
		}
	}

	invalid_proxies := []string{
		"fp-anhklb-20161214-001",
	}
	for _, proxy := range invalid_proxies {
		code, country, city := proxyLoc(proxy)
		assert.Empty(t, code)
		assert.Empty(t, country)
		assert.Empty(t, city)
	}
}
