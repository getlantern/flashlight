package proxied

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/anacrolix/publicip"
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

func getRandomInfohashes(t *testing.T, maxCount int) []string {
	a := []string{}
	for i := 0; i < maxCount; i++ {
		b := make([]byte, 40) // size doesn't matter
		_, err := cryptoRand.Read(b)
		require.NoError(t, err)
		sum := sha1.New().Sum(b)
		h := hex.EncodeToString(sum)
		a = append(a, h)
	}
	return a
}

// initP2pPeers runs the entire P2p flow between "Free" peers and "Censored" peers:
// - Make a new Free peer locally
//   - Have it instantiate a CONNECT proxy
//   - Have it announce a few random infohashes to all the common DHT bootstrap
//     nodes with the ip:port of the CONNECT proxy
// - Make a new Censored peer locally
//   - Using the same random infohashes, have it get the CONNECT proxy ip:ports
//     of the free peers using the DHT network
//   - Proxy an HTTP request through the CONNECT proxy and assert the result is
//     as expected
//
// All of this is done asynchronously to mimic how the application would really
// behave.
func initP2pPeers(
	t *testing.T,
	interceptConnectDial bool,
) (*p2p.FreeP2pCtx, *p2p.CensoredP2pCtx) {
	// Get a few random infohashes
	ihs := getRandomInfohashes(t, 3)

	pubIpv4, err := publicip.Get4(context.Background())
	require.NoError(t, err)
	// ipv6 is not so important and might not be always available

	// Setup and run a censored peer
	censoredP2pCtx, err := p2p.RunCensoredP2pLogic(
		0,
		ihs,
		interceptConnectDial,
		pubIpv4,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, censoredP2pCtx)

	// Setup and run a free peer
	freeP2pCtx, err := p2p.RunFreeP2pLogic(
		ihs,
		// registrarEndpoint. Will be done here later: https://github.com/getlantern/lantern-internal/issues/5235
		"",
		t.TempDir(),
		true,  // verbose
		0,     // port
		false, // portforwardWithUpnp. We don't need upnp in tests
	)
	require.NoError(t, err)
	require.NotNil(t, freeP2pCtx)
	return freeP2pCtx, censoredP2pCtx
}

type URLTestInput struct {
	url               string
	expectedSubstring string
}

func TestP2p(t *testing.T) {
	freeP2pCtx, censoredP2pCtx := initP2pPeers(t,
		false, // interceptConnectDial. Keep this false since we don't want to
		// intercept the CONNECT dials: we can use http.RoundTripper directly
	)
	t.Cleanup(func() {
		freeP2pCtx.Close(context.Background())
		censoredP2pCtx.Close(context.Background())
	})

	// Configure fronted package
	fronted.ConfigureForTest(t)
	fronted.ConfigureHostAlaisesForTest(t, map[string]string{
		// XXX <31-01-22, soltzen> This mapping was chosen for testing
		// since this API is a core component of Lantern and will likely
		// remain for a long time.
		"geo.getiantem.org": "d3u5fqukq7qrhd.cloudfront.net",
	})

	inputTestURLs := []URLTestInput{
		// TODO <06-06-2022, soltzen> We can't add the google.com domain since
		// there's no alias for it and simply adding google.com to the domain
		// list above is not sufficient
		// {
		// 	url:               "https://www.google.com/humans.txt",
		// 	expectedSubstring: "Google is built by a large team of engineers, designers, researchers, robots, and others in many different sites across the globe. It is updated continuously, and built with more tools and technologies than we can shake a stick at. If you'd like to help us out, see careers.google.com.",
		// },
		{
			url:               "http://geo.getiantem.org/lookup/95.90.211.100",
			expectedSubstring: "Germany",
		},
		{
			url:               "http://geo.getiantem.org/lookup/198.199.72.101",
			expectedSubstring: "United States",
		},
	}

	for _, tc := range []struct {
		name                string
		rt                  http.RoundTripper
		expectedTransportId TransportId
	}{
		{
			name:                "P2P",
			rt:                  p2p(censoredP2pCtx),
			expectedTransportId: TransportId_P2p,
		},
		{
			// This tests FrontedAndP2p's ability to run two roundtrippers, using
			// the result of the first one, and discarding the second, by delaying
			// of the roundtrippers at a time and testing if the next one would
			// yield a correct response
			name: "Delay fronted and assert p2p runs",
			rt: FrontedAndP2p(
				censoredP2pCtx, // p2pCtx
				5*time.Minute,  // masqueradeTimeout
				true,           // addDebugHeaders
				// Interception function
				func(transportId TransportId, cl *http.Client, req *http.Request) {
					// Delay p2p indefinitely
					if transportId == TransportId_Fronted {
						time.Sleep(9999999 * time.Minute)
					}
				},
			),
			expectedTransportId: TransportId_P2p,
		},
		{
			name: "Delay p2p and assert fronted runs",
			rt: FrontedAndP2p(
				censoredP2pCtx, // p2pCtx
				5*time.Minute,  // masqueradeTimeout
				true,           // addDebugHeaders
				// Interception function
				func(transportId TransportId, cl *http.Client, req *http.Request) {
					// Delay p2p indefinitely
					if transportId == TransportId_P2p {
						time.Sleep(9999999 * time.Minute)
					}
				},
			),
			expectedTransportId: TransportId_Fronted,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, testURL := range inputTestURLs {
				t.Run(testURL.url, func(t *testing.T) {
					req, err := http.NewRequest(
						"GET",
						testURL.url,
						nil)
					require.NoError(t, err)

					cl := &http.Client{
						Timeout:   60 * time.Second,
						Transport: tc.rt}
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
				})
			}
		})
	}
}
