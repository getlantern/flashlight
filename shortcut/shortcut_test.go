package shortcut

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/shortcut"
	"github.com/stretchr/testify/assert"
)

func TestShortcutResources(t *testing.T) {
	log := golog.LoggerFor("shortcut-test")
	countries := []string{"ae", "cn", "ir", "default"}

	var sc shortcut.Shortcut
	for _, country := range countries {
		sc = testCountry(t, country)
		log.Debugf("Initialized shortcut for '%s':\n\t%#v", country, sc)
	}
	method := func(addr string) shortcut.Method {
		method, _ := sc.RouteMethod(context.Background(), addr)
		return method
	}
	sc = testCountry(t, "ir")
	assert.Equal(t, shortcut.Proxy, method("10.10.1.1"))
	assert.Equal(t, shortcut.Direct, method("10.11.1.1"))
}

func testCountry(t *testing.T, country string) shortcut.Shortcut {
	v4, v4err := ipRanges.ReadFile("resources/" + country + "_ipv4.txt")
	assert.Nil(t, v4err)
	v6, v6err := ipRanges.ReadFile("resources/" + country + "_ipv6.txt")
	assert.Nil(t, v6err)
	v4Proxied, v4errProxied := ipRanges.ReadFile("resources/" + country + "_ipv4_proxied.txt")
	assert.Nil(t, v4errProxied)
	v6Proxied, v6errProxied := ipRanges.ReadFile("resources/" + country + "_ipv6_proxied.txt")
	assert.Nil(t, v6errProxied)
	sc := shortcut.NewFromReader(
		bytes.NewReader(v4),
		bytes.NewReader(v6),
		bytes.NewReader(v4Proxied),
		bytes.NewReader(v6Proxied),
	)
	return sc
}

func TestUnconfiguredCountry(t *testing.T) {
	v4, err := ipRanges.ReadFile("resources/default_ipv4.txt")
	assert.NoError(t, err)
	sc := shortcut.NewFromReader(
		bytes.NewReader(v4),
		bytes.NewReader([]byte("")),
		bytes.NewReader([]byte("")),
		bytes.NewReader([]byte("")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	method, _ := sc.RouteMethod(ctx, "10.10.1.1:80")
	assert.Equal(t, shortcut.Direct, method)
	cancel()

	configure("gb")
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	method, _ = Allow(ctx, "10.10.1.1:80")
	assert.Equal(t, shortcut.Direct, method)
	cancel()
}
