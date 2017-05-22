package config

import (
	"net/http"
	"net/http/httputil"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/stretchr/testify/assert"
)

type dumpRequestRT struct {
	wrapped http.RoundTripper
	t       *testing.T
}

func (rt dumpRequestRT) RoundTrip(req *http.Request) (*http.Response, error) {
	dump, _ := httputil.DumpRequestOut(req, false)
	rt.t.Log(string(dump))
	return rt.wrapped.RoundTrip(req)
}

func TestFetchGlobal(t *testing.T) {
	// global config is hosted on S3, use the standard ETag
	testFetch(t, false, globalChained, globalFronted)
}

func TestFetchProxies(t *testing.T) {
	// fetching proxies goes all the way to config-server
	testFetch(t, true, proxiesChained, proxiesFronted)
}

// testFetch actually fetches a config file over the network.
func testFetch(t *testing.T, useLanternEtag bool, chained string, fronted string) {
	rt := dumpRequestRT{&http.Transport{}, t}
	cf := newFetcher(rt, useLanternEtag,
		common.WrapUserConfig(
			func() int64 { return 1 },
			func() string { return "token" },
		), chained, fronted)

	bytes, err := cf.fetch()
	assert.Nil(t, err)
	assert.True(t, len(bytes) > 200)

	lastETag := cf.(*fetcher).lastCloudConfigETag[chained]
	t.Log(lastETag)
	assert.NotNil(t, lastETag, "should save ETag")
	bytes, err = cf.fetch()
	assert.Nil(t, err)
	assert.Nil(t, bytes, "fetching again should get unchanged content")
}
