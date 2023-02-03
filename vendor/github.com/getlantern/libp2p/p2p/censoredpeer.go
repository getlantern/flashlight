package p2p

import (
	"context"
	stdErrors "errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/libp2p/common"
	"github.com/getlantern/libp2p/dhtwrapper"
	"github.com/getlantern/libp2p/logger"
	"github.com/getlantern/libp2p/peersrepository"
	"github.com/getlantern/quicproxy"
	"github.com/pkg/errors"
)

const defaultPullFreePeersWaitDurationIfNoErr = 30 * time.Minute
const defaultPullFreePeersWaitDurationIfErr = 10 * time.Second
const defaultNumForwardProxyServers = 5

var (
	ErrNoResponseFromAnyPeer = stdErrors.New("no response from any peer")
)

// CensoredPeerCtx is the context for a censored peer.
//
// Use it directly with CensoredPeerCtx as an http.RoundTripper
//
//     cl := &http.Client{
//     	Timeout:   60 * time.Second,
//     	Transport: censoredPeerCtx}
//     resp, err := cl.Do(req)
//     dieIfErr(err)
//
// The flow of traffic with this mode goes like this from the perspective of
// each receiver/sender:
//
// 1. The Client
//   - This is the user of this structure
//   - Client uses CensoredPeerCtx as an http.RoundTripper (as show above)
//   - CensoredPeerCtx.RoundTrip() is triggered when a request is made
// 2. The ForwardProxy
//   - Now the ForwardProxy has just received a CONNECT request from the Client
//     to proxy some traffic.
//   - It proxies the request to the **address of the ReverseProxy**
//     - In the case of QUIC as a dialer, this is done in
//       quicproxy.forwardproxy#NewforwardProxy()
//     - The address of the ReverseProxy is set using
//       quicproxy.forwardproxy#SetReverseProxyUrl(), which is called inside
//       CensoredPeerCtx.RoundTrip() after a suitable FreePeer is found
// 3. The ReverseProxy
//   - The ReverseProxy has just received a CONNECT request from the ForwardProxy
//   - Based on whether it's HTTP or HTTPs, it'll be handled by
//     quicproxy.reverseproxy.ConnectDial and
//     quicproxy.reverseproxy.NonproxyHandler respectively
//
//
// XXX <06-09-2022, soltzen> There was another "proxy" mode where you can set
// your browser (or any tool that supports HTTP proxies) to use the
// ForwardProxy as a proxy. Something like this
// `curl --proxy http://localhost:FORWARD_PROXY_PORT http://reddit.com`.
// This is no longer supported. The code was written and tested but removed to
// keep the project simple. See the PR in this issue where the code was
// removed: https://github.com/getlantern/lantern-internal/issues/5631
type CensoredPeerCtx struct {
	PeersRepo *peersrepository.PeersRepository

	input *CensoredPeerCtxInput

	// Pool of QuicForwardProxy and a queue that holds indices of each forward
	// proxy in the slice so that it each can be used only one at a time
	forwardProxyServerPool      []*quicproxy.QuicForwardProxy
	forwardProxyServerPoolQueue chan int

	// doneChan is only used for its closed state and it's closed when this
	// peer is closed and no other work from this struct should continue
	doneChan chan struct{}
	doneOnce sync.Once
}

type CensoredPeerCtxInput struct {
	Bep44TargetsAndSalts []common.Bep44TargetAndSalt
	DHTWrapper           dhtwrapper.DHTWrapper
	// Ipv4Addr, Ipv6Addr are optional parameters (i.e., they can be nil) that
	// represent the public IP addresses of the censored peer. They are used in
	// PeersRepository to determine if a FreePeer and a CensoredPeer are on the
	// same network, which would significantly reduce the complexity of their
	// communication since the two peers can use the loopback interface instead
	// of public ips to talk to each other.
	Ipv4Addr, Ipv6Addr net.IP

	PullFreePeersWaitDurationIfNoErr time.Duration
	PullFreePeersWaitDurationIfErr   time.Duration

	// If nil, use defaultTryRequestThroughPeer()
	TryRequestFunc RequestThroughPeerTrier

	// Number of forward proxies to run in parallel. If 0, use
	// defaultNumForwardProxyServers. For an explanation on why we need
	// multiple forward proxies, see the comment in RoundTrip().
	NumForwardProxyServers int
}

func (i *CensoredPeerCtxInput) parseAndSetDefaults() error {
	if i.TryRequestFunc == nil {
		i.TryRequestFunc = defaultTryRequestThroughPeer
	}
	if i.PullFreePeersWaitDurationIfNoErr == 0 {
		i.PullFreePeersWaitDurationIfNoErr = defaultPullFreePeersWaitDurationIfNoErr
	}
	if i.PullFreePeersWaitDurationIfErr == 0 {
		i.PullFreePeersWaitDurationIfErr = defaultPullFreePeersWaitDurationIfErr
	}
	if i.NumForwardProxyServers == 0 {
		i.NumForwardProxyServers = defaultNumForwardProxyServers
	}
	return nil
}

// RunCensoredPeer creates a CensoredPeerCtx and runs it.
func RunCensoredPeer(
	input *CensoredPeerCtxInput,
) (*CensoredPeerCtx, error) {
	logger.Log.Infof("Running a new CensoredPeerCtx")
	if err := input.parseAndSetDefaults(); err != nil {
		return nil, err
	}

	// Collect all the important struct into a new CensoredPeerCtx
	cpc := &CensoredPeerCtx{
		input:     input,
		doneChan:  make(chan struct{}),
		PeersRepo: peersrepository.NewPeersRepository2(5),
	}

	// for each Bep44Target, start pulling it asynchronously continuously
	// TODO <28-07-2022, soltzen> This does not account for dynamic changes to
	// the targets (i.e., if the targets were updated in global-config or
	// wherever we fetch the target from)
	for _, ts := range input.Bep44TargetsAndSalts {
		go startPullFreePeersRoutine(
			input.DHTWrapper,
			ts,
			input.PullFreePeersWaitDurationIfNoErr,
			input.PullFreePeersWaitDurationIfErr,
			cpc.doneChan,
			cpc.PeersRepo,
		)
	}

	// Run a bunch of forward proxies. See the comment in RoundTrip() for an
	// explanation on why we need multiple forward proxies.
	errChan := make(chan error, input.NumForwardProxyServers)
	cpc.forwardProxyServerPoolQueue = make(
		chan int,
		input.NumForwardProxyServers,
	)
	for i := 0; i < input.NumForwardProxyServers; i++ {
		srv, err := quicproxy.NewForwardProxy(
			true, // verbose
			// insecureSkipVerify
			// TODO <21-02-22, soltzen> For the POC, this is fine, but in
			// production, it'll be great if the reverse proxy we're connecting to
			// has a certificate signed by a Lantern CA, which we can also add here.
			// Or, we can just perform cert pinning with the public key of the
			// reverse proxy obtained through the DHT. See here:
			// https://github.com/getlantern/lantern-internal/issues/5230
			true,
			nil,
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"Unable to create forward proxy server",
			)
		}
		if err := srv.Run(0, errChan); err != nil {
			return nil, errors.Wrapf(err, "Unable to run forward proxy server")
		}
		logger.Log.Infof(
			"Successfully created a forward proxy for a censored p2p peer on localhost:%v",
			srv.Port,
		)
		cpc.forwardProxyServerPool = append(cpc.forwardProxyServerPool, srv)
		// Load its ID into the queue
		cpc.forwardProxyServerPoolQueue <- i
	}

	// Monitor errors for all the forward proxy servers
	go func() {
		for err := range errChan {
			logger.Log.Errorf("while running CensoredPeer: %v", err)
			ctx, cancel := context.WithTimeout(context.Background(),
				5*time.Second)
			cpc.Close(ctx)
			cancel()
		}
	}()

	return cpc, nil
}

// RoundTrip makes CensoredPeerCtx implement the http.RoundTripper interface.
// The process is as follows:
// 1. Get all unique FreePeers as of this moment from the PeersRepo.
//   - If there are no FreePeers, GetUniqueFreePeers() will block until there
//     is at least one.
// 2. For each FreePeer, try to proxy "req" through it. If the request
//    succeeds, return the response and cancel the other requests.
// 3. If all requests fail, return all unique errors.
// 4. If the request's context is canceled, return the context's error.
//
// ## Why we need multiple forward proxies
//
// We wanna run multiple requests simultaneously to the FreePeers.
//
// In practice, a request is proxied from a CensoredPeer to a FreePeer to the
// free internet like this:
//
// CensoredPeer
//   -> CensoredPeer's ForwardProxy
//     -> FreePeer's ReverseProxy
//       -> Internet
//
// The request "req" here will not be proxied directly from the CensoredPeer's
// forward proxy to the Internet. Instead, it will be proxied from the
// CensoredPeer's forward proxy to the **FreePeer's reverse proxy**, which will
// then proxy the request to the Internet.
//
// What this means is that all HTTP CONNECT requests will **need to be
// forwarded** from the CensoredPeer's forward proxy to the FreePeer's reverse
// proxy. Forwarding a CONNECT request is done as follows:
//
//   QuicForwardProxy.SetReverseProxyUrl(
//     fmt.Sprintf(
//       "http://%s:%d",
//       freePeer.IP.To4().String(),
//       freePeer.Port))
//
// So we **cannot** use **one** forward proxy to proxy all requests to the
// FreePeers. Each forward proxy needs to be assigned a **unique** FreePeer to
// proxy requests to. This is why we need multiple forward proxies.
func (cpc *CensoredPeerCtx) RoundTrip(
	req *http.Request,
) (*http.Response, error) {
	logger.Log.Infof(
		"CensoredPeer: Running RoundTrip for req: %s",
		req.URL.String(),
	)

	// Find all unique peers
	freePeers, err := cpc.PeersRepo.GetUniqueFreePeers(req.Context())
	if err != nil {
		logger.Log.Errorf(
			"CensoredPeer: while getting unique free peers: %v",
			err,
		)
		return nil, errors.Wrap(err, "while getting unique peers")
	}
	if freePeers == nil {
		logger.Log.Errorf("CensoredPeer: no free peers found")
		return nil, errors.New("no free peers found")
	}
	logger.Log.Infof(
		"CensoredPeer: Found %v unique free peers",
		len(freePeers),
	)

	respChan := make(chan *http.Response, len(freePeers))
	errChan := make(chan error, len(freePeers))
	for _, _peer := range freePeers {
		peer := replaceIPWithLoopbackIfLocal(
			_peer,
			cpc.input.Ipv4Addr,
			cpc.input.Ipv6Addr,
		)

		// Clone request
		childReq := req.Clone(req.Context())

		go func() {
			// Get a forward proxy server from the pool and release it after
			// we're done
			srvIdx, srv := cpc.GetForwardProxyServer(req.Context())
			defer cpc.ReleaseForwardProxyServer(srvIdx)

			// Run the request through the forward proxy
			resp, err := cpc.input.TryRequestFunc(childReq, peer, srv)
			if err != nil {
				errChan <- err
				return
			}
			respChan <- resp
		}()
	}

	// Wait for one of the following:
	// - A response
	// - The request's context to be cancelled
	// - For all the requests to fail
	errMap := make(map[string]error)
	attempts := 0
	for {
		select {
		case resp := <-respChan:
			return resp, nil
		case err := <-errChan:
			errMap[err.Error()] = err
			attempts++
			if attempts == len(freePeers) {
				sb := strings.Builder{}
				for _, err := range errMap {
					sb.WriteString(err.Error())
					sb.WriteString("; ")
				}
				return nil, errors.Wrapf(
					ErrNoResponseFromAnyPeer,
					sb.String(),
				)
			}
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

type RequestThroughPeerTrier func(
	req *http.Request,
	p *common.GenericFreePeer,
	forwardProxySrv *quicproxy.QuicForwardProxy,
) (*http.Response, error)

func defaultTryRequestThroughPeer(
	req *http.Request,
	p *common.GenericFreePeer,
	forwardProxySrv *quicproxy.QuicForwardProxy,
) (*http.Response, error) {
	// Make an http.Client that uses the forward proxy
	s := "http://localhost:" + strconv.Itoa(forwardProxySrv.Port)
	proxyUrl, err := url.Parse(s)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to parse proxy URL: %v", s)
	}
	cl := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)},
	}

	// Dial this peer when forwardProxyServer receives a CONNECT
	// request
	forwardProxySrv.SetReverseProxyUrl(fmt.Sprintf("http://%s:%d", p.IP.To4().String(), p.Port))

	// Just as a summary, the flow of this request, starting from this point,
	// goes like this for https connections:
	// - The "cl" pointer above runs "req"
	// - Before running the request, a CONNECT request is sent to
	//   cpc.ForwardProxyServer (i.e., our QuicForwardProxy)
	// - QuicForwardProxy receives the CONNECT request and dials the "p"
	//   peer, as specified in the forwardProxySrv.SetReverseProxyUrl call
	//   above.
	// - QuicReverseProxy, running on the FreePeer side, receives the
	//   CONNECT request and does its job
	// - If all goes well, QuicReverseProxy responds to the CONNECT request
	//   with a 200 OK (done transparently by Go's net/http library)
	// - Now, "req" is sent to the "p" peer, and the response is returned
	resp, err := cl.Do(req)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"while running request using peer [%v] as proxy",
			p,
		)
	}
	return resp, nil
}

// Close shutsdown this peer's resources.
func (cpc *CensoredPeerCtx) Close(ctx context.Context) {
	cpc.doneOnce.Do(func() {
		logger.Log.Infof("CensoredPeer: Closing...")
		for _, srv := range cpc.forwardProxyServerPool {
			if srv == nil {
				continue
			}
			err := srv.Shutdown(ctx)
			if err != nil {
				logger.Log.Errorf(
					"while closing forward proxy server: %v",
					err,
				)
			}
		}
		close(cpc.doneChan)
	})
}

// replaceIPWithLoopbackIfLocal checks if this peer is on localhost. If so,
// assign localhost for the IP.
//
// We mainly do this to avoid a situation where a CensoredPeer and a FreePeer
// that are on the same public IP talk to each other **through** their public
// IPs. The FreePeer becomes unrecognized to the CensoredPeer if you set it as
// its proxy. In practice, this case mostly happens in local developer testing.
//
// TODO <01-06-2022, soltzen> Should we do this also for IPs in
// the same subnet?
func replaceIPWithLoopbackIfLocal(
	p *common.GenericFreePeer,
	publicIPv4, publicIPv6 net.IP,
) *common.GenericFreePeer {
	if publicIPv4 != nil && p.IP.String() == publicIPv4.String() {
		p.IP = net.IPv4(127, 0, 0, 1)
	}
	if publicIPv6 != nil && p.IP.String() == publicIPv6.String() {
		p.IP = net.IPv6loopback
	}
	return p
}

func (cpc *CensoredPeerCtx) GetForwardProxyServer(
	ctx context.Context,
) (int, *quicproxy.QuicForwardProxy) {
	select {
	case id := <-cpc.forwardProxyServerPoolQueue:
		return id, cpc.forwardProxyServerPool[id]
	case <-ctx.Done():
		return 0, nil
	}
}

func (cpc *CensoredPeerCtx) ReleaseForwardProxyServer(id int) {
	cpc.forwardProxyServerPoolQueue <- id
}
