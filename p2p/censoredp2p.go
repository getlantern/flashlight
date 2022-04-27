package p2p

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/getlantern/golog"
	"github.com/getlantern/quicproxy"
)

var log = golog.LoggerFor("flashlight-p2p")

// Set this to true if all the peers you're expecting to work with (censored
// and free) are on localhost. This is useful only for testing
var AreAllPeersOnLocalhost = false

const defaultCensoredPeerProxyingRoundtripTimeout = 10 * time.Second
const defaultMaxPeers = 1024
const defaultMaxRetriesOnFailedRoundTrip = 3
const defaultRetryDelayOnFailedRoundTrip = 5 * time.Second

// CensoredP2pCtx is the context for a censored peer.
// There're two ways to funnel traffic through this object so it'll reach a free peer:
//
// - as an http.Transport. Use it like this:
//   - with proxied.P2p()
//
// 				cl := &http.Client{
// 					Timeout:   60 * time.Second,
// 					Transport: proxied.P2p(censoredP2pCtx)}
// 				resp, err := cl.Do(req)
// 				require.NoError(t, err)
//
//   - with proxied.FrontedAndP2p()
//
// 				cl := &http.Client{
// 					Timeout:   60 * time.Second,
// 					Transport: proxied.P2p(censoredP2pCtx)}
// 				resp, err := cl.Do(req)
// 				require.NoError(t, err)
//
//
// - Another way is to connect to it directly. See
// [here](https://github.com/getlantern/replica-p2p/blob/689120875c42fcd4094b7af195909f27c30b4c9e/cmd/censoredpeer/main.go#L43)
// for more info. This is really only useful if you want to use this censored
// peer from outside of Go code, where you can't specify an http.Transport, but
// only a proxy address (e.g., curl -x http://my.censoredpeer.proxy.addr https://google.com)
type CensoredP2pCtx struct {
	dhtServer                   *dht.Server
	infohashes                  [][20]byte
	PeersRepo                   *PeersRepository
	PubIp                       string
	roundtripTimeout            time.Duration
	replicaP2pFunctions         ReplicaP2pFunctions
	retryDelayOnFailedRoundTrip time.Duration
	maxRetriesOnFailedRoundTrip int
	closeOnce                   sync.Once
	closeChan                   chan struct{}
	ForwardProxyServer          *quicproxy.QuicForwardProxy
}

// NewCensoredP2pCtx creates a new context for a censored peer.
//
// - replicaP2pFunctions is the interface that glues DHT functions that live in
//   getlantern/replica without embedding getlantern/replica package in
//   flashlight
//
// Leave maxPeers, roundtripTimeout, maxDurationToKeepUnusedPeers
// maxRetriesOnFailedRoundTrip, retryDelayOnFailedRoundTrip and
// flushUnusedPeersDelay as 0 if you want to use the default values.
//
// TODO <18-01-22, soltzen> This only deals with ipv4
func NewCensoredP2pCtx(
	peerInfoHashes []string,
	replicaP2pFunctions ReplicaP2pFunctions,
	roundtripTimeout time.Duration,
	maxPeers int,
	maxDurationToKeepUnusedPeers time.Duration,
	flushUnusedPeersDelay time.Duration,
	retryDelayOnFailedRoundTrip time.Duration,
	maxRetriesOnFailedRoundTrip int,
) (*CensoredP2pCtx, error) {
	// Init DHT server
	cfg := dht.NewDefaultServerConfig()
	cfg.NoSecurity = false
	// XXX <18-01-22, soltzen> Censored peers are always passive
	cfg.Passive = true
	s, err := dht.NewServer(cfg)
	if err != nil {
		return nil, log.Errorf("%v", err)
	}

	// Convert infohashes from []string -> [][20]byte
	ihs := [][20]byte{}
	for _, v := range peerInfoHashes {
		var ih [20]byte
		h, err := hex.DecodeString(v)
		if err != nil {
			return nil, log.Errorf("%v", err)
		}
		copy(ih[:], h)
		ihs = append(ihs, ih)
	}

	// Init peers repo
	pr := NewPeersRepository(
		maxPeers,
		maxDurationToKeepUnusedPeers,
		flushUnusedPeersDelay,
	)
	pr.StartCollectionAndFlushRoutines()

	if roundtripTimeout == 0 {
		roundtripTimeout = defaultCensoredPeerProxyingRoundtripTimeout
	}
	if maxPeers == 0 {
		maxPeers = defaultMaxPeers
	}
	if maxDurationToKeepUnusedPeers == 0 {
		maxDurationToKeepUnusedPeers = defaultMaxDurationToKeepUnusedPeers
	}
	if flushUnusedPeersDelay == 0 {
		flushUnusedPeersDelay = defaultFlushUnusedPeersDelay
	}
	if maxRetriesOnFailedRoundTrip == 0 {
		maxRetriesOnFailedRoundTrip = defaultMaxRetriesOnFailedRoundTrip
	}
	if retryDelayOnFailedRoundTrip == 0 {
		retryDelayOnFailedRoundTrip = defaultRetryDelayOnFailedRoundTrip
	}

	return &CensoredP2pCtx{
		dhtServer:                   s,
		infohashes:                  ihs,
		replicaP2pFunctions:         replicaP2pFunctions,
		PeersRepo:                   pr,
		roundtripTimeout:            roundtripTimeout,
		maxRetriesOnFailedRoundTrip: maxRetriesOnFailedRoundTrip,
		retryDelayOnFailedRoundTrip: retryDelayOnFailedRoundTrip,
		closeChan:                   make(chan struct{}),
	}, nil
}

func (p2pCtx *CensoredP2pCtx) StartForwardProxy(
	port int,
	interceptConnectDial bool,
	errChan chan<- error,
) error {
	s, err := quicproxy.NewForwardProxy(
		true, // verbose
		// insecureSkipVerify
		// TODO <21-02-22, soltzen> For the POC, this is fine, but in
		// production, it'll be great if the reverse proxy we're connecting to
		// has a certificate signed by a Lantern CA, which we can also add here.
		// Or, we can just perform cert pinning with the public key of the
		// reverse proxy obtained through the DHT. See here:
		// https://github.com/getlantern/lantern-internal/issues/5230
		true,
	)
	if err != nil {
		return log.Errorf("%v", err)
	}
	if interceptConnectDial {
		p2pCtx.interceptConnectDial(s)
	}

	err = s.Run(port, errChan)
	if err != nil {
		return log.Errorf("%v", err)
	}
	p2pCtx.ForwardProxyServer = s
	return nil
}

func (p2pCtx *CensoredP2pCtx) interceptConnectDial(proxy *quicproxy.QuicForwardProxy) {
	proxy.Proxy.ConnectDial = func(
		ctx context.Context,
		network, addr string,
	) (c net.Conn, err error) {
		// On a CONNECT request, loop through all the possible peers we
		// have
		p2pCtx.PeersRepo.Loop(ctx, func(p *Peer) bool {
			// Dial to that proxy
			u := fmt.Sprintf("http://%s:%d", p.IP.String(), p.Port)
			log.Debugf("Next free peer to try to CONNECT to: %v", u)
			c, err = proxy.Proxy.NewConnectDialToProxy(u)(ctx, network, addr)
			if err != nil {
				log.Debugf("Error dialing to proxy. Trying the next peer: %v", err)
				// See if we exceeded our context deadline
				select {
				case <-ctx.Done():
					err = ctx.Err()
					// We've already assigned the error: we can exit safely
					return true
				default:
					// Loop and try the next one
					p2pCtx.PeersRepo.Drop(p)
					return false
				}
			}
			// Set active peer and exit successfully
			p2pCtx.PeersRepo.UsedPeer = p
			return true
		})
		return
	}
}

// GetPeers attempts to fetch peers for the assigned infohashes in
// CensoredP2pCtx.infohashes.
func (p2pCtx *CensoredP2pCtx) GetPeers(ctx context.Context) error {
	log.Debugf("Censored peer: Attempting to get peers for infohashes %+v",
		p2pCtx.infohashes)
	return p2pCtx.replicaP2pFunctions.GetPeers(
		ctx,
		[]*dht.Server{p2pCtx.dhtServer},
		p2pCtx.infohashes, p2pCtx.PeersRepo.C)
}

// RoundTrip is the only function in http.RoundTripper interface.
// It does the following:
// - Try to proxy a request with the latest peer we connected through, if any
// - Else, loop through all the peers we collected so far (by calling CensoredP2pCtx.GetPeer())
//   - And attempt to connect through them
//   - Return an error if we failed to connect through any free peer
//   - Else, mark the last successful peer and return the response
// - In all cases, drop all peers we failed to connect through
func (p2pCtx *CensoredP2pCtx) RoundTrip(req *http.Request) (*http.Response, error) {
	log.Debugf("Censored peer: Running RoundTrip for req: %s", req.URL.String())
	tryRequestWithPeer := func(req *http.Request, p *Peer) (*http.Response, error) {
		// Dial this peer when p2pCtx.ForwardProxyServer receives a CONNECT
		// request
		p2pCtx.ForwardProxyServer.SetReverseProxyUrl(
			fmt.Sprintf("http://%s:%d", p.IP.To4().String(), p.Port))

		proxyUrl, err := url.Parse("http://localhost:" + strconv.Itoa(p2pCtx.ForwardProxyServer.Port))
		if err != nil {
			return nil, log.Errorf("during url parsing for peer [%v]: %v", p, err)
		}
		cl := &http.Client{
			Timeout: p2pCtx.roundtripTimeout,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
			},
		}
		// The flow of this request, starting from this point, goes like this:
		// - The "cl" pointer above runs "req"
		// - Before running the request, a CONNECT request is sent to
		//   p2pCtx.ForwardProxyServer (i.e., our QuicForwardProxy)
		// - QuicForwardProxy receives the CONNECT request and dials the "p"
		//   peer, as specified in the
		//   p2pCtx.ForwardProxyServer.Proxy.ConnectDial assignment above
		// - QuicReverseProxy, running on the FreePeer side, receives the
		//   CONNECT request and does its job
		// - If all goes well, QuicReverseProxy responds to the CONNECT request
		//   with a 200 OK (done transparently by Go's net/http library)
		// - Now, "req" is sent to the "p" peer, and the response is returned
		resp, err := cl.Do(req)
		if err != nil {
			return nil, log.Errorf("while running request using peer [%v] as proxy: %v", p, err)
		}
		return resp, nil
	}

	var resp *http.Response
	// Try to find any peer to connect from all available peers in each retry.
	// If we found no peers to connect to, wait a bit and retry the loop again.
	attemptAndRetryIfFailed(
		p2pCtx.maxRetriesOnFailedRoundTrip,
		p2pCtx.retryDelayOnFailedRoundTrip,
		func(attemptNumber int) bool {
			log.Debugf("Attempting to proxy request through any available free peer (Attempt #%d)", attemptNumber)
			// Returning true means "Continue looping through the available peers."
			// Returning false will terminate the loop
			p2pCtx.PeersRepo.Loop(req.Context(), func(p *Peer) bool {
				var err error
				resp, err = tryRequestWithPeer(req, p)
				if err != nil {
					log.Debugf(
						"Censored peer: Error while proxying through a p2p peer [%+v]. Trying the next one: %v", p, err)
					p2pCtx.PeersRepo.Drop(p)
					// Try the next peer
					return false
				}
				log.Debugf("Censored peer: Proxied request %s through peer %s successfully",
					req.URL, p.IP)
				p2pCtx.PeersRepo.UsedPeer = p
				// We've connected with one: let's keep using it until it doesn't work
				// anymore
				return true
			})

			// Retry if we got no response
			if resp == nil {
				return false
			}
			// Else, break and exit
			return true
		},
	)

	if resp == nil {
		return nil, errors.New("Failed to find a free peer to connect through")
	}
	return resp, nil
}

// attemptAndRetryIfFailed calls function 'f'. If 'f' returns false, wait
// 'retryDelay' time units and retry. This will repeat 'maxRetries' times
// before we exit.
func attemptAndRetryIfFailed(maxRetries int, retryDelay time.Duration, f func(int) bool) {
	for i := maxRetries; i > 0; i-- {
		if f(maxRetries - i) {
			break
		}
		time.Sleep(retryDelay)
	}
}

// Close shutsdown this peer's resources.
func (p2pCtx *CensoredP2pCtx) Close(ctx context.Context) {
	p2pCtx.closeOnce.Do(func() {
		log.Debugf("Censored peer: Closing...")
		if p2pCtx.dhtServer != nil {
			p2pCtx.dhtServer.Close()
		}
		if p2pCtx.PeersRepo != nil {
			p2pCtx.PeersRepo.Close()
		}
		if p2pCtx.ForwardProxyServer != nil {
			err := p2pCtx.ForwardProxyServer.Shutdown(ctx)
			if err != nil {
				log.Debugf("Error while closing forward proxy server: %v", err)
			}
		}

		close(p2pCtx.closeChan)
	})
}

func (p2pCtx *CensoredP2pCtx) IsClosed() <-chan struct{} {
	return p2pCtx.closeChan
}
