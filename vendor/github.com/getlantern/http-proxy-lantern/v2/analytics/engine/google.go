package engine

import (
	"github.com/siddontang/go/log"
	"net/url"
)

const (
	// endpoint is the endpoint to report GA data to.
	gaEndpoint = `https://ssl.google-analytics.com/collect`
)

type googleAnalytics struct {
	id string
}

func NewGA(trackingID string) Engine {
	return &googleAnalytics{id: trackingID}
}

func (ga googleAnalytics) GetID() string {
	return ga.id
}

func (ga googleAnalytics) GetEndpoint() string {
	return gaEndpoint
}

func (ga googleAnalytics) GetSessionValues(sa *SessionParams, site string, port string) string {
	vals := make(url.Values)

	// Version 1 of the API
	vals.Add("v", "1")
	// Our Google Tracking ID
	vals.Add("tid", sa.TrackingID)
	// The client's ID (Lantern DeviceID, which is Base64 encoded 6 bytes from mac
	// address)
	vals.Add("cid", sa.ClientId)

	// Override the users IP so we get accurate geo data.
	// vals.Add("uip", ip)
	vals.Add("uip", sa.IP)
	// Make call to anonymize the user's IP address -- basically a policy thing where
	// Google agrees not to store it.
	vals.Add("aip", "1")

	// Track this as a page view
	vals.Add("t", "pageview")

	// Track custom port dimension
	vals.Add("cd1", port)

	log.Tracef("Tracking view to site: %v using GA", site)
	vals.Add("dp", site)

	// Use the user-agent reported by the client
	vals.Add("ua", sa.UserAgent)

	// Use the server's hostname as the campaign source so that we can track
	// activity per server
	vals.Add("cs", sa.Hostname)
	// Campaign medium and campaign name are required for campaign tracking to do
	// anything. We just fill them in with some dummy values.
	vals.Add("cm", "proxy")
	vals.Add("cn", "proxy")

	// Note the absence of session tracking. We don't have a good way to tell
	// when a session ends, so we don't bother with it.
	return vals.Encode()
}
