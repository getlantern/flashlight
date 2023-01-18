package engine

import (
	"github.com/siddontang/go/log"
	"net/url"
)

const (
	// endpoint is the endpoint to report Matomo data to.
	matomoEndpoint  = `https://lantern.matomo.cloud/matomo.php` // - this should be in an env
	matomoIDSite    = "1"                                       // lantern.io - this should be in an env
	matomoAuthToken = "06111c9cb3eb8b065d3f0af3d400ca8b"        // this should be in an env
)

type matomo struct{}

func NewMatomo() Engine {
	return &matomo{}
}

func (ma matomo) GetID() string {
	return matomoIDSite
}

func (ma matomo) GetEndpoint() string {
	return matomoEndpoint
}

func (ma matomo) GetSessionValues(sa *SessionParams, site string, port string) string {
	vals := make(url.Values)

	// Version 1 of the API
	vals.Add("apiv", "1")
	vals.Add("rec", "1")
	vals.Add("token_auth", matomoAuthToken)
	// Our Matomo Site ID
	vals.Add("idsite", matomoIDSite)
	// The client's ID (Lantern DeviceID, which is Base64 encoded 6 bytes from mac
	// address)
	vals.Add("cid", sa.ClientId)
	// Override the users IP so we get accurate geo data.
	// vals.Add("cip", ip)
	vals.Add("cip", sa.IP)

	// Track this as a page view
	vals.Add("action_name", "pageview")

	// Track custom port dimension
	vals.Add("dimension1", port)

	log.Tracef("Tracking view to site: %v using Matomo", site)
	vals.Add("url", site)

	// Use the user-agent reported by the client
	vals.Add("ua", sa.UserAgent)

	// Use the server's hostname as the campaign source so that we can track
	// activity per server
	vals.Add("utm_source", sa.Hostname)
	// Campaign medium and campaign name are required for campaign tracking to do
	// anything. We just fill them in with some dummy values.
	vals.Add("utm_medium", "proxy")
	vals.Add("utm_campaign", "proxy")

	// Note the absence of session tracking. We don't have a good way to tell
	// when a session ends, so we don't bother with it.
	return vals.Encode()
}
