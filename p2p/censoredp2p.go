package p2p

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("flashlight-p2p")

// Set this to true if all the peers you're expecting to work with (censored
// and free) are on localhost. This is useful only for testing
var AreAllPeersOnLocalhost = false

const defaultCensoredPeerProxyingRoundtripTimeout = 10 * time.Second
const defaultMaxPeers = 1024
const defaultMaxRetriesOnFailedRoundTrip = 3
const defaultRetryDelayOnFailedRoundTrip = 5 * time.Second

type CensoredP2pCtx struct {
	dhtServer                   *dht.Server
	infohashes                  [][20]byte
	PeersRepo                   *PeersRepository
	PubIp                       string
	roundtripTimeout            time.Duration
	httpClient                  *http.Client
	replicaP2pFunctions         ReplicaP2pFunctions
	retryDelayOnFailedRoundTrip time.Duration
	maxRetriesOnFailedRoundTrip int
	closeOnce                   sync.Once
	closeChan                   chan struct{}
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
		httpClient:                  &http.Client{Timeout: roundtripTimeout},
		closeChan:                   make(chan struct{}),
	}, nil
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
	op := ops.Begin("censored_peer_p2p_roundtripper")
	defer op.End()
	log.Debugf("Censored peer: Running RoundTrip for req: %s", req.URL.String())
	tryRequestWithPeer := func(req *http.Request, p *Peer) (*http.Response, error) {
		if AreAllPeersOnLocalhost {
			log.Debugf("Censored peer: Detected that the free peer we're trying to proxy through is in the same LAN as us. This can happen during tests. Changing the free peer's IP to localhost to avoid router confusion")
			p.IP = net.IPv4(127, 0, 0, 1)
		}

		proxyUrl, err := url.Parse("http://" + p.String())
		if err != nil {
			return nil, log.Errorf("during url parsing for peer [%v]: %v",
				p, op.FailIf(err))
		}
		p2pCtx.httpClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
		}
		startTime := time.Now()
		resp, err := p2pCtx.httpClient.Do(req)
		if err != nil {
			return nil, log.Errorf("while running request using peer [%v] as proxy: %v",
				p, op.FailIf(err))
		}
		op.SetMetricAvg("rtt_in_ms", float64(time.Since(startTime).Milliseconds()))
		op.SetMetricAvg("rtt_content_length", float64(resp.ContentLength))
		return resp, nil
	}

	// Try the last successful peer we connected with, if any
	if p2pCtx.PeersRepo.UsedPeer != nil {
		log.Debugf("Censored Peer: Trying last successful peer: %s", p2pCtx.PeersRepo.UsedPeer.IP)
		resp, err := tryRequestWithPeer(req, p2pCtx.PeersRepo.UsedPeer)
		if err == nil {
			log.Debugf(
				"Censored peer: Successfully proxied a request through our last successful p2p peer [%+v]",
				p2pCtx.PeersRepo.UsedPeer)
			return resp, nil
		}
		log.Debugf(
			"Censored peer: Failed to connect with our last peer [%+v]. Attempting to use another one",
			p2pCtx.PeersRepo.UsedPeer)
		p2pCtx.PeersRepo.Remove(p2pCtx.PeersRepo.UsedPeer)
		p2pCtx.PeersRepo.UsedPeer = nil
	}

	var resp *http.Response
	// Try to find any peer to connect from all available peers in each retry.
	// If we found no peers to connect to, wait a bit and retry the loop again.
	attemptAndRetryIfFailed(
		p2pCtx.maxRetriesOnFailedRoundTrip,
		p2pCtx.retryDelayOnFailedRoundTrip,
		func(attemptNumber int) bool {
			log.Debugf("Attempting to proxy request through any available free peer (Attempt #%d)", attemptNumber)
			peersToDrop := []*Peer{}
			// Returning true means "Continue looping through the available peers."
			// Returning false will terminate the loop
			p2pCtx.PeersRepo.Loop(func(p *Peer) bool {
				var err error
				resp, err = tryRequestWithPeer(req, p)
				if err != nil {
					log.Debugf("Censored peer: Error while trying to proxy through a p2p peer [%+v]. Trying the next one: %v", p, err)
					peersToDrop = append(peersToDrop, p)
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

			// Remove peers that never worked from our repository
			// XXX <18-01-22, soltzen> We're removing peers away outside of
			// `p2pCtx.PeersRepo.Loop()` since we have an RLock inside Loop()
			// and we'll have another lock while removing elements. It's best
			// to do those two operations separately to avoid races
			if len(peersToDrop) != 0 {
				log.Debugf("Attempting to drop %d peers", len(peersToDrop))
			}
			for _, p := range peersToDrop {
				p2pCtx.PeersRepo.Remove(p)
			}

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

func (p2pCtx *CensoredP2pCtx) Close() {
	p2pCtx.closeOnce.Do(func() {
		log.Debugf("Censored peer: Closing...")
		if p2pCtx.dhtServer != nil {
			p2pCtx.dhtServer.Close()
		}
		if p2pCtx.PeersRepo != nil {
			p2pCtx.PeersRepo.Close()
		}
		close(p2pCtx.closeChan)
	})
}

func (p2pCtx *CensoredP2pCtx) IsClosed() <-chan struct{} {
	return p2pCtx.closeChan
}
