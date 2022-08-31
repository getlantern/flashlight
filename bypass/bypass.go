package bypass

// bypass periodically sends traffic to the bypass blocking detection server. The server uses the ratio
// between domain fronted and proxied traffic to determine if proxies are blocked. The client randomizes
// the intervals between calls to the server and also randomizes the length of requests.
import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	mrand "math/rand"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-cloud/cmd/api/apipb"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"
)

var log = golog.LoggerFor("bypass")

// The way lantern-cloud is configured, we need separate URLs for domain fronted vs proxied traffic.
const dfEndpoint = "https://iantem.io/api/v1/bypass"
const proxyEndpoint = "https://api.iantem.io/v1/bypass"

type bypass struct {
	infos     map[string]*apipb.ProxyConfig
	proxies   []*proxy
	mxProxies sync.Mutex
}

// Start sends periodic traffic to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start(listen func(func(map[string]*apipb.ProxyConfig, config.Source)), configDir string, userConfig common.UserConfig) func() {
	mrand.Seed(time.Now().UnixNano())
	b := &bypass{
		infos:   make(map[string]*apipb.ProxyConfig),
		proxies: make([]*proxy, 0),
	}
	listen(func(infos map[string]*apipb.ProxyConfig, src config.Source) {
		b.OnProxies(infos, configDir, userConfig)
	})
	return b.reset
}

func (b *bypass) OnProxies(infos map[string]*apipb.ProxyConfig, configDir string, userConfig common.UserConfig) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	b.reset()
	dialers := chained.CreateDialersMap(configDir, infos, userConfig)
	for k, v := range infos {
		dialer := dialers[k]
		if dialer == nil {
			log.Errorf("No dialer for %v", k)
			continue
		}

		pc := chained.CopyConfig(v)
		// Set the name in the info since we know it here.
		pc.Name = k
		// Kill the cert to avoid it taking up unnecessary space.
		pc.Cert = ""
		p := b.newProxy(k, pc, configDir, userConfig, dialer)
		b.proxies = append(b.proxies, p)
		go p.start()
	}
}

func (b *bypass) newProxy(name string, pc *apipb.ProxyConfig, configDir string, userConfig common.UserConfig, dialer balancer.Dialer) *proxy {
	return &proxy{
		ProxyConfig:       pc,
		name:              name,
		done:              make(chan bool),
		toggle:            atomic.NewBool(mrand.Float32() < 0.5),
		dfRoundTripper:    proxied.Fronted(0),
		userConfig:        userConfig,
		proxyRoundTripper: proxyRoundTripper(name, pc, configDir, userConfig, dialer),
	}
}

func (b *bypass) reset() {
	for _, v := range b.proxies {
		v.stop()
	}
	b.proxies = make([]*proxy, 0)
}

type proxy struct {
	*apipb.ProxyConfig
	name              string
	done              chan bool
	randString        string
	dfRoundTripper    http.RoundTripper
	proxyRoundTripper http.RoundTripper
	configDir         string
	toggle            *atomic.Bool
	userConfig        common.UserConfig
}

func (p *proxy) start() {
	log.Debugf("Starting bypass for proxy %v", p.name)
	p.callRandomly(p.sendToBypass)
}

func (p *proxy) sendToBypass() int64 {
	// We alternate between domain fronting and proxying to ensure that, in aggregate, we
	// send both equally. We avoid sending both a domain fronted and a proxied request
	// in rapid succession to avoid the blocking detection itself being a signal.
	var rt http.RoundTripper
	var endpoint string
	if p.toggle.Toggle() {
		log.Debug("Using proxy directly")
		rt = p.proxyRoundTripper
		endpoint = proxyEndpoint
	} else {
		rt = p.dfRoundTripper
		log.Debug("Using domain fronting")
		endpoint = dfEndpoint
	}

	req, err := p.newRequest(p.userConfig, endpoint)
	if err != nil {
		log.Errorf("Unable to create request: %v", err)
		return 0
	}

	log.Debugf("Sending traffic for bypass server: %v", p.name)
	resp, err := rt.RoundTrip(req)
	if err != nil || resp == nil {
		log.Errorf("Unable to post chained server info: %v", err)
		return 0
	}
	defer func() {
		if resp.Body != nil {
			if closeerr := resp.Body.Close(); closeerr != nil {
				log.Errorf("Error closing response body: %v", closeerr)
			}
		}
	}()
	if resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
	}

	var sleepTime int64
	sleepVal := resp.Header.Get(common.SleepHeader)
	if sleepVal != "" {
		sleepTime, err = strconv.ParseInt(sleepVal, 10, 64)
		if err != nil {
			log.Errorf("Could not parse sleep val: %v", err)
		}
	}
	if resp.StatusCode != http.StatusOK {
		log.Errorf("Unexpected response code %v: for response %#v", resp.Status, resp)
		// If we don't get a 200, we'll revert to the default sleep time.
		return -1
	} else {
		log.Debugf("Successfully got response from: %v", p.name)
	}
	return sleepTime
}

func proxyRoundTripper(name string, info *apipb.ProxyConfig, configDir string, userConfig common.UserConfig, dialer balancer.Dialer) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		log.Debugf("Dialing chained server at: %s", addr)
		pc, _, err := dialer.DialContext(ctx, balancer.NetworkConnect, addr)
		if err != nil {
			log.Errorf("Unable to dial chained server: %v", err)
		} else {
			log.Debug("Successfully dialed chained server")
		}
		return pc, err
	}
	return transport
}

func (p *proxy) newRequest(userConfig common.UserConfig, endpoint string) (*http.Request, error) {
	// Just posting all the info about the server allows us to control these fields fully on the server
	// side.
	infopb, err := proto.Marshal(p.ProxyConfig)
	if err != nil {
		log.Errorf("Unable to marshal chained server info: %v", err)
		return nil, err
	}

	if err != nil {
		log.Errorf("Unable to write chained server info: %v", err)
		return nil, err
	}

	log.Debugf("Creating request for endpoint: %v", endpoint)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(infopb))
	if err != nil {
		log.Errorf("Unable to create request: %v", err)
		return nil, err
	}
	common.AddCommonHeaders(userConfig, req)

	// make sure to close the connection after reading the Body this prevents the occasional
	// EOFs errors we're seeing with successive requests
	req.Close = true
	req.Header.Set("Content-Type", "application/x-protobuf")
	log.Debug("Sending request")

	return req, nil
}

func (p *proxy) stop() {
	p.done <- true
}

// callRandomly calls the given function at a random interval between 2 and 7 minutes, unless
// the provided function overrides the default sleep.
func (p *proxy) callRandomly(f func() int64) {
	calls := atomic.NewInt64(0)
	var sleepTime int64
	var elapsed time.Duration
	var sleep = func() <-chan time.Time {
		defer func() {
			calls.Inc()
		}()
		base := 20
		// If we just started up, we want to send traffic a little quicker to make sure we factor in users
		// that don't run for very long.
		if calls.Load() == 0 {
			log.Debug("Making first call sooner")
			base = 3
		}
		var delay time.Duration
		if sleepTime > 0 {
			delay = time.Duration(sleepTime) * time.Second
		}
		delay = elapsed + (time.Duration(base*2+mrand.Intn(base*5)) * time.Second)
		log.Debugf("Next call in %v", delay)
		return time.After(delay)
	}

	for {
		select {
		case <-p.done:
			return
		case <-sleep():
			start := time.Now()
			sleepTime = f()
			elapsed = time.Since(start)
		}
	}
}
