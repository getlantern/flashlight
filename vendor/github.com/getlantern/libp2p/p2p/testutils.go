package p2p

import (
	"context"
	"net"

	"github.com/anacrolix/publicip"
	"github.com/getlantern/libp2p/common"
	"github.com/getlantern/libp2p/dhtwrapper"
	"github.com/getlantern/libp2p/testutils"
	"github.com/stretchr/testify/require"
)

type InitTestPeersInput struct {
	DontReturnFreePeers bool
	// Leave empty to not use the registrar at all
	RegistrarEndpoint string
}

var inputTestPeersSaneDefaults = &InitTestPeersInput{
	DontReturnFreePeers: false,
}

// InitTestP2PPeers runs the entire P2p flow between "Free" peers and "Censored"
// peers:
// - Make a new Free peer locally
//   - opens a reverse proxy, ready for connections
// - Make a new Censored peer locally
//   - opens a forward proxy in "RoundTripper" mode, making CensoredPeerCtx
//     ready to be used as an http.RoundTripper
//
// One major difference to a real world scenario: we're skipping the peer
// discovery through the DHT (done by the p2pregistrar after a FreePeer
// registers to it by running an HTTP request to its /register endpoint). The
// FreePeer is **not** communicating with the p2pregistrar.
// This is done to make this test deterministic and not depend on a live
// p2pregistrar instance.
//
// All of this is done asynchronously to mimic how the application would really
// behave. This function call is not blocking.
func InitTestPeers(
	t require.TestingT,
	tempDir string,
	input *InitTestPeersInput,
) (*FreePeerCtx, *CensoredPeerCtx) {
	// Sanitize input
	if input == nil {
		input = inputTestPeersSaneDefaults
	}
	// Setup and run a free peer
	pubIpv4, err := publicip.Get4(context.Background())
	require.NoError(t, err)
	fpc, err := RunFreePeer(
		&FreePeerCtxInput{
			Port:        0,
			GetIpv4Addr: func() (net.IP, error) { return pubIpv4, nil },
			GetIpv6Addr: nil, // not important
			Verbose:     true,
			// We're gonna talk locally: no need to expose anything through our
			// NAT
			DoPortforwardWithUpnp: false,
			AppConfigPath:         tempDir,
			RegistrarEndpoint:     input.RegistrarEndpoint,
			// TODO <02-08-2022, soltzen> Hardcoding these domains is not
			// ideal. Maybe have a "*" wildcard that accepts everything. For
			// now, though, this covers all our test cases
			AllowedDomainsToProxyTo: []string{
				"google", "getiantem",
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, fpc)

	// Setup and run a censored peer
	// Define a dhtup.Resource that responds with the FreePeer
	// IP:Port when asked
	freePeer, err := fpc.ToGenericFreePeer()
	require.NoError(t, err)

	var dhtWrap dhtwrapper.DHTWrapper
	if input.DontReturnFreePeers {
		dhtWrap = &testutils.DHTWrapperMock__DontReturnFreePeers{}
	} else {
		dhtWrap = &testutils.DHTWrapperMock__AlwaysReturnPeers{
			FreePeers: []*common.GenericFreePeer{freePeer},
		}
	}
	cpc, err := RunCensoredPeer(
		&CensoredPeerCtxInput{
			// Bep44TargetsAndSalts doesn't matter as long as it is not empty.
			// DHTWrapperMock__AlwaysReturnPeers will return "freePeer"
			// regardless of the target
			Bep44TargetsAndSalts: common.GenerateRandomizedBep44TargetAndSalt(t, 1),
			DHTWrapper:           dhtWrap,
			Ipv4Addr:             pubIpv4,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, cpc)
	return fpc, cpc
}
