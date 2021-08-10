package geolookup

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/getlantern/fronted"
	"github.com/stretchr/testify/require"
)

func getPublicIpFromIdent(t *testing.T) string {
	// This service just returns your public IP upon a GET request
	resp, err := http.Get("http://ident.me")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestFronted(t *testing.T) {
	fronted.ConfigureHostAlaisesForTest(t, map[string]string{
		"geo.getiantem.org": "d3u5fqukq7qrhd.cloudfront.net",
	})
	ch := DefaultInstance.OnRefresh()
	DefaultInstance.Refresh()
	country := DefaultInstance.GetCountry(60 * time.Second)
	ip := DefaultInstance.GetIP(5 * time.Second)
	require.Equal(t, 2, len(country), "Bad country '%v' for ip %v", country, ip)
	require.Equal(t, getPublicIpFromIdent(t), ip, "Bad IP %s", ip)
	require.True(t, <-ch, "should update watcher")
}
