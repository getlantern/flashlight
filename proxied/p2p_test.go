package proxied

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/getlantern/fronted"
	p2pLogger "github.com/getlantern/libp2p/logger"
	"github.com/getlantern/libp2p/p2p"
	"github.com/getlantern/quicproxy"
	"github.com/stretchr/testify/require"
)

func init() {
	quicproxy.Log = quicproxy.StdLogger{}
	p2pLogger.Log = p2pLogger.StdLogger{}
}

type URLTestInput struct {
	url               string
	expectedSubstring string
}

// TestP2PRoundTrippers tests P2P roundtrippers in the proxied package under different
// conditions. A few important notes:
// - The DHT is effectively bypassed in this test: we're initializing a real
//   FreePeer and a CensoredPeer, but the discovery occurs through a mock
//   (namely, using `p2p.MockResource__Success`, where the CensoredPeer
//   automatically obtains the FreePeer's address.
// - The FreePeer doesn't run any UPNP: all the communications between the two
//   peers are done in the local loopback interface. This is important because
//   real life constraints (e.g., the FreePeer's NAT is not accessible from the
//   CensoredPeer) do happen and this test is not accounting for it.
// - Lastly, since we're skipping the peer discovery through the DHT (done by
//   the p2pregistrar after a FreePeer registers to it by running an HTTP request
//   to its /register endpoint), the FreePeer is **not** communicating with the
//   p2pregistrar.
//
// It would be interesting to make a test that runs _just_ a CensoredPeer
// against a collection of always-live FreePeers (like the demo FreePeers in
// libp2p:
// https://github.com/getlantern/libp2p/blob/d87b308752b67559dd71db66842ded03b9ff1721/README.md#L103)
func TestP2PRoundTrippers(t *testing.T) {
	freeP2pCtx, censoredP2pCtx := p2p.InitTestP2PPeers(t)
	t.Cleanup(func() {
		freeP2pCtx.Close(context.Background())
		censoredP2pCtx.Close(context.Background())
	})

	// Configure fronted package
	fronted.ConfigureForTest(t)
	fronted.ConfigureHostAlaisesForTest(t, map[string]string{
		// XXX <31-01-22, soltzen> This API is a core component of Lantern and
		// will likely remain for a long time. It's safe to use for testing
		"geo.getiantem.org": "d3u5fqukq7qrhd.cloudfront.net",
	})

	for _, tc := range []struct {
		name                               string
		initRoundTripper                   func() http.RoundTripper
		expectedSucceedingRoundTripperName FlowComponentID
		inputTestURLs                      []URLTestInput
	}{
		{
			name: "P2P",
			initRoundTripper: func() http.RoundTripper {
				return NewProxiedFlow(&ProxiedFlowInput{AddDebugHeaders: true}).
					Add(FlowComponentID_P2P, censoredP2pCtx, false)

			},
			expectedSucceedingRoundTripperName: FlowComponentID_P2P,
			inputTestURLs: []URLTestInput{
				{
					url:               "https://www.google.com/humans.txt",
					expectedSubstring: "Google is built by a large team of engineers, designers, researchers, robots, and others in many different sites across the globe. It is updated continuously, and built with more tools and technologies than we can shake a stick at. If you'd like to help us out, see careers.google.com.",
				},
				{
					url:               "http://geo.getiantem.org/lookup/95.90.211.100",
					expectedSubstring: "Germany",
				},
				{
					url:               "http://geo.getiantem.org/lookup/198.199.72.101",
					expectedSubstring: "United States",
				},
			},
		},

		{
			name: "P2P and Fronted: preferred P2P",
			initRoundTripper: func() http.RoundTripper {
				return NewProxiedFlow(&ProxiedFlowInput{AddDebugHeaders: true}).
					Add(FlowComponentID_P2P, censoredP2pCtx, true).
					Add(FlowComponentID_Fronted, Fronted(0), false)

			},
			expectedSucceedingRoundTripperName: FlowComponentID_P2P,
			inputTestURLs: []URLTestInput{
				{
					url:               "http://geo.getiantem.org/lookup/95.90.211.100",
					expectedSubstring: "Germany",
				},
				{
					url:               "http://geo.getiantem.org/lookup/198.199.72.101",
					expectedSubstring: "United States",
				},
			},
		},

		{
			name: "P2P and Fronted: preferred Fronted",
			initRoundTripper: func() http.RoundTripper {
				return NewProxiedFlow(&ProxiedFlowInput{AddDebugHeaders: true}).
					Add(FlowComponentID_Fronted, Fronted(0), true).
					Add(FlowComponentID_P2P, censoredP2pCtx, false)

			},
			expectedSucceedingRoundTripperName: FlowComponentID_P2P,
			inputTestURLs: []URLTestInput{
				{
					url:               "http://geo.getiantem.org/lookup/95.90.211.100",
					expectedSubstring: "Germany",
				},
				{
					url:               "http://geo.getiantem.org/lookup/198.199.72.101",
					expectedSubstring: "United States",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, testURL := range tc.inputTestURLs {
				t.Run(testURL.url, func(t *testing.T) {
					req, err := http.NewRequest(
						"GET",
						testURL.url,
						nil)
					require.NoError(t, err)

					cl := &http.Client{
						Timeout:   60 * time.Second,
						Transport: tc.initRoundTripper()}
					resp, err := cl.Do(req)
					require.NoError(t, err)

					require.Equal(t, http.StatusOK, resp.StatusCode)
					defer resp.Body.Close()
					b, err := ioutil.ReadAll(resp.Body)
					require.NoError(t, err)
					require.Contains(
						t,
						string(b),
						testURL.expectedSubstring,
					)
					// Expect the correct roundtripper to have succeeded
					require.Equal(
						t,
						tc.expectedSucceedingRoundTripperName,
						resp.Header.Get(roundTripperHeaderKey),
					)
				})
			}
		})
	}

}
