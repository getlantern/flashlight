package client

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/chained"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/idletiming"
	"github.com/getlantern/netx"
	"github.com/getlantern/withtimeout"
)

var (
	// Close connections idle for a period to avoid dangling connections. 45 seconds
	// is long enough to avoid interrupt normal connections but shorter than the
	// idle timeout on the server to avoid running into closed connection problems.
	// 45 seconds is also longer than the MaxIdleTime on our http.Transport, so it
	// doesn't interfere with that.
	idleTimeout = 45 * time.Second

	// ForceChainedProxyAddr - If specified, all proxying will go through this address
	ForceChainedProxyAddr string

	// ForceAuthToken - If specified, auth token will be forced to this
	ForceAuthToken string
)

// ChainedServerInfo contains all the data for connecting to a given chained
// server.
type ChainedServerInfo struct {
	// Addr: the host:port of the upstream proxy server
	Addr string

	// Cert: optional PEM encoded certificate for the server. If specified,
	// server will be dialed using TLS over tcp. Otherwise, server will be
	// dialed using plain tcp. For OBFS4 proxies, this is the Base64-encoded obfs4
	// certificate.
	Cert string

	// AuthToken: the authtoken to present to the upstream server.
	AuthToken string

	// Trusted: Determines if a host can be trusted with plain HTTP traffic.
	Trusted bool

	// PluggableTransport: If specified, a pluggable transport will be used
	PluggableTransport string

	// PluggableTransportSettings: Settings for pluggable transport
	PluggableTransportSettings map[string]string
}

// ChainedDialer creates a *balancer.Dialer backed by a chained server.
func ChainedDialer(si *ChainedServerInfo, deviceID string, proTokenGetter func() string) (*balancer.Dialer, error) {
	s, err := newServer(si)
	if err != nil {
		return nil, err
	}
	return s.dialer(deviceID, proTokenGetter)
}

type chainedServer struct {
	*ChainedServerInfo
	df dialFactory
}

func newServer(si *ChainedServerInfo) (*chainedServer, error) {
	if si.PluggableTransport != "" {
		log.Debugf("Using pluggable transport %v for server at %v", si.PluggableTransport, si.Addr)
	}

	df := pluggableTransports[si.PluggableTransport]
	if df == nil {
		return nil, fmt.Errorf("No dial factory defined for transport: %v", si.PluggableTransport)
	}

	s := &chainedServer{ChainedServerInfo: si,
		df: df,
	}

	return s, nil
}

func (s *chainedServer) dialer(deviceID string, proTokenGetter func() string) (*balancer.Dialer, error) {
	dial, err := s.df(s.ChainedServerInfo, deviceID)
	if err != nil {
		return nil, fmt.Errorf("Unable to construct dialFN: %v", err)
	}

	// Is this a trusted proxy that we could use for HTTP traffic?
	var trusted string
	if s.Trusted {
		trusted = "(trusted) "
	}
	label := fmt.Sprintf("%schained proxy at %s [%v]", trusted, s.Addr, s.PluggableTransport)

	ccfg := chained.Config{
		DialServer: dial,
		Label:      label,
		OnRequest: func(req *http.Request) {
			s.attachHeaders(req, deviceID, proTokenGetter)
		},
	}
	d := chained.NewDialer(ccfg)
	return &balancer.Dialer{
		Label:   label,
		Trusted: s.Trusted,
		DialFN: func(network, addr string) (net.Conn, error) {
			var conn net.Conn
			var err error

			op := ops.Begin("dial_for_balancer").ProxyType(ops.ProxyChained).ProxyAddr(s.Addr)
			defer op.End()

			if addr == s.Addr {
				// Check if we are trying to connect to our own server and bypass proxying if so
				// This accounts for the case w/ multiple instances of Lantern running on mobile
				// Whenever full-device VPN mode is enabled, we need to make sure we ignore proxy
				// requests from the first instance.
				log.Debugf("Attempted to dial ourselves. Dialing directly to %s instead", addr)
				conn, err = netx.DialTimeout(network, addr, 1*time.Minute)
			} else {
				// Yeah any site visited through Lantern can be a check target
				balancer.AddCheckTarget(addr)
				conn, err = d(network, addr)
			}

			if err != nil {
				return nil, op.FailIf(err)
			}
			conn = idletiming.Conn(conn, idleTimeout, func() {
				log.Debugf("Proxy connection to %s via %s idle for %v, closed", addr, conn.RemoteAddr(), idleTimeout)
			})
			return conn, nil
		},
		Check: func(checkData interface{}, onFailure func(string)) (bool, time.Duration) {
			return s.check(d, checkData.([]string), deviceID, proTokenGetter, onFailure)
		},
		OnRequest: ccfg.OnRequest,
	}, nil
}

func (s *chainedServer) attachHeaders(req *http.Request, deviceID string, proTokenGetter func() string) {
	authToken := s.AuthToken
	if ForceAuthToken != "" {
		authToken = ForceAuthToken
	}
	if authToken != "" {
		req.Header.Add("X-Lantern-Auth-Token", authToken)
	} else {
		log.Errorf("No auth token for request to %v", req.URL)
	}
	req.Header.Set("X-Lantern-Device-Id", deviceID)
	if token := proTokenGetter(); token != "" {
		req.Header.Set("X-Lantern-Pro-Token", token)
	}
}

// check pings the 10 most popular sites in the user's history
func (s *chainedServer) check(dial func(string, string) (net.Conn, error),
	urls []string, deviceID string,
	proTokenGetter func() string,
	onFailure func(string)) (bool, time.Duration) {
	rt := &http.Transport{
		DisableKeepAlives: true,
		Dial:              dial,
	}
	allPassed := true
	totalLatency := time.Duration(0)
	for _, url := range urls {
		start := time.Now()
		// We ping the URLs through the proxy to get timings
		req, err := http.NewRequest("GET", "http://ping-chained-server", nil)
		if err != nil {
			log.Errorf("Could not create HTTP request: %v", err)
			return false, 0
		}
		req.Header.Set("X-Lantern-PingURL", url)
		// We set X-Lantern-Ping in case we're hitting an old http-server that
		// doesn't support pinging URLs.
		req.Header.Set("X-Lantern-Ping", "small")

		s.attachHeaders(req, deviceID, proTokenGetter)
		ok, timedOut, _ := withtimeout.Do(60*time.Second, func() (interface{}, error) {
			resp, err := rt.RoundTrip(req)
			if err != nil {
				log.Debugf("Error testing dialer %s to %s: %s", s.Addr, url, err)
				return false, nil
			}
			if resp.Body != nil {
				// Read the body to include this in our timing.
				// Note - for bandwidth saving reasons, the server may not send the body
				// but if it does, we'll read it.
				defer resp.Body.Close()
				_, err = io.Copy(ioutil.Discard, resp.Body)
				if err != nil {
					return false, fmt.Errorf("Unable to read response body: %v", err)
				}
			}
			log.Tracef("PING %s through chained server at %s, status code %d", url, s.Addr, resp.StatusCode)
			success := resp.StatusCode >= 200 && resp.StatusCode <= 299
			if success {
				delta := time.Duration(time.Now().Sub(start))
				if strings.HasSuffix(s.PluggableTransport, "kcp") && inChina() {
					// Heavily bias kcp results to essentially force kcp protocol
					delta = time.Duration(float64(delta) / 10)
				}
				totalLatency += delta
			} else {
				onFailure(url)
			}
			return success, nil
		})
		if timedOut || !ok.(bool) {
			if timedOut {
				log.Errorf("Timed out checking dialer at: %v", s.Addr)
			}
			allPassed = false
		}
	}

	return allPassed, totalLatency
}

func inChina() bool {
	return geolookup.GetCountry(50*time.Millisecond) == "CN"
}
