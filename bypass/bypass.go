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

	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/apipb"
	"github.com/getlantern/flashlight/v7/bandit"
	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("bypass")

	// some pluggable transports don't work with bypass
	unsupportedTransports = map[string]bool{
		"broflake": true,
	}
)

// The way lantern-cloud is configured, we need separate URLs for domain fronted vs proxied traffic.
const (
	dfEndpoint    = "https://iantem.io/api/v1/bypass"
	proxyEndpoint = "https://api.iantem.io/v1/bypass"

	// version is the bypass client version. It is not necessary to update this value on every
	// change to bypass; this should only be updated when the backend needs to make decisions unique
	// to a new version of bypass.
	version int32 = 1
)

type bypass struct {
	infos     map[string]*commonconfig.ProxyConfig
	proxies   []*proxy
	mxProxies sync.Mutex
}

// Start sends periodic traffic to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start(listen func(func(map[string]*commonconfig.ProxyConfig, config.Source)), configDir string, userConfig common.UserConfig) func() {
	mrand.Seed(time.Now().UnixNano())
	b := &bypass{
		infos:   make(map[string]*commonconfig.ProxyConfig),
		proxies: make([]*proxy, 0),
	}
	listen(func(infos map[string]*commonconfig.ProxyConfig, src config.Source) {
		b.OnProxies(infos, configDir, userConfig)
	})
	return b.reset
}

func (b *bypass) OnProxies(infos map[string]*commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	b.reset()

	// Some pluggable transports don't support bypass, filter these out here.
	supportedInfos := make(map[string]*commonconfig.ProxyConfig, len(infos))

	for k, v := range infos {
		if !unsupportedTransports[v.PluggableTransport] {
			supportedInfos[k] = v
		}
	}

	dialers := chained.CreateDialersMap(configDir, supportedInfos, userConfig)
	for name, config := range supportedInfos {
		dialer := dialers[name]
		if dialer == nil {
			log.Errorf("No dialer for %v", name)
			continue
		}

		// if dialer is not ready, try to load it async
		ready, err := dialer.IsReady()
		if err != nil {
			log.Errorf("dialer %q isn't ready and returned an error: %w", name, err)
			continue
		}
		if !ready {
			log.Debugf("dialer %q is not ready, starting in background", name)
			go b.loadProxyAsync(name, config, configDir, userConfig, dialer)
			continue
		}

		b.startProxy(name, config, configDir, userConfig, dialer)
	}
}

func (b *bypass) loadProxyAsync(proxyName string, config *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer bandit.Dialer) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	readyChan := make(chan struct{})
	retry := atomic.NewBool(true)
	go func() {
		for retry.Load() {
			time.Sleep(15 * time.Second)
			ready, err := dialer.IsReady()
			if err != nil {
				log.Errorf("dialer %q isn't ready and returned an error: %w", proxyName, err)
				cancel()
				break
			}
			if !ready {
				b.startProxy(proxyName, config, configDir, userConfig, dialer)
				readyChan <- struct{}{}
				break
			}
		}
	}()
	select {
	case _, ok := <-readyChan:
		if !ok {
			log.Errorf("ready channel for proxy %q is closed", proxyName)
		}
	case <-ctx.Done():
		log.Errorf("proxy %q took to long to get ready", proxyName)
	}
	retry.Store(false)
	close(readyChan)
}

func (b *bypass) startProxy(proxyName string, config *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer bandit.Dialer) {
	pc := chained.CopyConfig(config)
	// Set the name in the info since we know it here.
	pc.Name = proxyName
	// Kill the cert to avoid it taking up unnecessary space.
	pc.Cert = ""
	p := b.newProxy(proxyName, pc, configDir, userConfig, dialer)
	b.proxies = append(b.proxies, p)
	go p.start()
}

func (b *bypass) newProxy(name string, pc *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer bandit.Dialer) *proxy {
	return &proxy{
		ProxyConfig:       pc,
		name:              name,
		done:              make(chan bool),
		toggle:            atomic.NewBool(mrand.Float32() < 0.5),
		dfRoundTripper:    proxied.Fronted("bypass_fronted_roundtrip", 0),
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
	*commonconfig.ProxyConfig
	name              string
	done              chan bool
	dfRoundTripper    http.RoundTripper
	proxyRoundTripper http.RoundTripper
	toggle            *atomic.Bool
	userConfig        common.UserConfig
}

func (p *proxy) start() {
	log.Debugf("Starting bypass for proxy %v", p.name)
	p.callRandomly(p.sendToBypass)
}

func (p *proxy) sendToBypass() int64 {
	op := ops.Begin("bypass_dial")
	defer op.End()

	// We alternate between domain fronting and proxying to ensure that, in aggregate, we
	// send both equally. We avoid sending both a domain fronted and a proxied request
	// in rapid succession to avoid the blocking detection itself being a signal.
	var rt http.RoundTripper
	var endpoint string
	var fronted bool
	if p.toggle.Toggle() {
		log.Debug("Using proxy directly")
		rt = p.proxyRoundTripper
		endpoint = proxyEndpoint
		fronted = false
	} else {
		rt = p.dfRoundTripper
		log.Debug("Using domain fronting")
		endpoint = dfEndpoint
		fronted = true
	}
	op.Set("fronted", fronted)

	req, err := p.newRequest(p.userConfig, endpoint)
	if err != nil {
		op.FailIf(log.Errorf("Unable to create request: %v", err))
		return 0
	}

	log.Debugf("Sending traffic for bypass server: %v", p.name)
	resp, err := rt.RoundTrip(req)
	if err != nil || resp == nil {
		op.FailIf(log.Errorf("Unable to post chained server info: %v", err))
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
		log.Errorf("Unexpected response code %v: fronted: %v for response %#v", resp.Status, fronted, resp)
	} else {
		log.Debugf("Successfully got response from: %v", p.name)
	}
	return sleepTime
}

func proxyRoundTripper(name string, info *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer bandit.Dialer) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		log.Debugf("Dialing chained server at: %s", addr)
		pc, _, err := dialer.DialContext(ctx, bandit.NetworkConnect, addr)
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
	bypassRequest := &apipb.BypassRequest{
		Config: &apipb.LegacyConnectConfig{
			Name:                       p.ProxyConfig.Name,
			Addr:                       p.ProxyConfig.Addr,
			Cert:                       p.ProxyConfig.Cert,
			PluggableTransport:         p.ProxyConfig.PluggableTransport,
			PluggableTransportSettings: p.ProxyConfig.PluggableTransportSettings,
			Location: &apipb.LegacyConnectConfig_ProxyLocation{
				City:        p.ProxyConfig.Location.GetCity(),
				Country:     p.ProxyConfig.Location.GetCountry(),
				CountryCode: p.ProxyConfig.Location.GetCountryCode(),
				Latitude:    p.ProxyConfig.Location.GetLatitude(),
				Longitude:   p.ProxyConfig.Location.GetLongitude(),
			},
			Track:  p.ProxyConfig.Track,
			Region: p.ProxyConfig.Region,
		},
		Version: version,
	}

	infopb, err := proto.Marshal(bypassRequest)
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

	var sleep = func(extraSleepTime int64, elapsed time.Duration) <-chan time.Time {
		defer func() {
			calls.Inc()
		}()
		base := 40
		// If we just started up, we want to send traffic a little quicker to make sure we factor in users
		// that don't run for very long.
		if calls.Load() == 0 {
			log.Debug("Making first call sooner")
			base = 3
		}
		var delay = elapsed + (time.Duration(base*2+mrand.Intn(base*5)) * time.Second)
		delay = delay + time.Duration(extraSleepTime)*time.Second
		log.Debugf("Next call in %v", delay)
		return time.After(delay)
	}

	// This is passed back from the server to add longer sleeps if desired.
	var extraSleepTime int64
	var elapsed time.Duration
	for {
		select {
		case <-p.done:
			return
		case <-sleep(extraSleepTime, elapsed):
			start := time.Now()
			extraSleepTime = f()
			elapsed = time.Since(start)
		}
	}
}
