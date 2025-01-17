package services

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	mrand "math/rand"

	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	commonconfig "github.com/getlantern/common/config"

	"github.com/getlantern/flashlight/v7/apipb"
	"github.com/getlantern/flashlight/v7/chained"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/dialer"
)

// bypass periodically sends traffic to the bypass blocking detection server. The server uses the ratio
// between domain fronted and proxied traffic to determine if proxies are blocked. The client randomizes
// the intervals between calls to the server and also randomizes the length of requests.

// The way lantern-cloud is configured, we need separate URLs for domain fronted vs proxied traffic.
const (
	dfEndpoint    = "https://iantem.io/api/v1/bypass"
	proxyEndpoint = "https://api.iantem.io/v1/bypass"

	// bypassSendInterval is the interval between sending traffic to the bypass server.
	bypassSendInterval = 4 * time.Minute

	// version is the bypass client version. It is not necessary to update this value on every
	// change to bypass; this should only be updated when the backend needs to make decisions unique
	// to a new version of bypass.
	version int32 = 1
)

var (
	// some pluggable transports don't work with bypass
	unsupportedTransports = map[string]bool{
		"broflake": true,
	}
)

type bypassService struct {
	infos     map[string]*commonconfig.ProxyConfig
	proxies   []*proxy
	mxProxies sync.Mutex
	// done is closed to notify the proxy bypass goroutines to stop.
	done chan struct{}
	// running is used to signal that the bypass service is running.
	running *atomic.Bool
}

// StartBypassService sends periodic traffic to the bypass server. The client periodically sends
// traffic to the server both via domain fronting and proxying to determine if proxies are blocked.
// StartBypassService returns a function to stop the service.
func StartBypassService(
	listen func(func(map[string]*commonconfig.ProxyConfig, config.Source)),
	configDir string,
	userConfig common.UserConfig,
) StopFn {
	b := &bypassService{
		infos:   make(map[string]*commonconfig.ProxyConfig),
		proxies: make([]*proxy, 0),
		done:    make(chan struct{}),
		running: atomic.NewBool(true),
	}

	logger.Debug("Starting bypass service")
	listen(func(infos map[string]*commonconfig.ProxyConfig, src config.Source) {
		b.onProxies(infos, configDir, userConfig)
	})
	return b.Stop
}

func (b *bypassService) onProxies(
	infos map[string]*commonconfig.ProxyConfig,
	configDir string,
	userConfig common.UserConfig,
) {
	if !b.Reset() {
		return // bypassService was stopped
	}

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
			logger.Errorf("No dialer for %v", name)
			continue
		}

		readyCh := dialer.Ready()
		if readyCh != nil {
			go b.loadProxyAsync(name, config, configDir, userConfig, dialer)
			continue
		}
		b.startProxy(name, config, configDir, userConfig, dialer)
	}
}

func (b *bypassService) loadProxyAsync(proxyName string, config *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer dialer.ProxyDialer) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	readyChan := make(chan struct{})
	go func() {
		dialerReady := dialer.Ready()
		if dialerReady == nil {
			b.startProxy(proxyName, config, configDir, userConfig, dialer)
			readyChan <- struct{}{}
			return
		}
		select {
		case err := <-dialerReady:
			if err != nil {
				logger.Errorf("dialer %q initialization failed: %w", proxyName, err)
				cancel()
				return
			}
			b.startProxy(proxyName, config, configDir, userConfig, dialer)
			readyChan <- struct{}{}
			return
		case <-ctx.Done():
			logger.Errorf("proxy %q took to long to start: %w", proxyName, ctx.Err())
			return
		}
	}()
	select {
	case <-readyChan:
		logger.Debugf("proxy ready!")
	case <-ctx.Done():
		logger.Errorf("proxy %q took to long to start: %w", proxyName, ctx.Err())
	}
}

func (b *bypassService) startProxy(proxyName string, config *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer dialer.ProxyDialer) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	pc := chained.CopyConfig(config)
	// Set the name in the info since we know it here.
	pc.Name = proxyName
	// Kill the cert to avoid it taking up unnecessary space.
	pc.Cert = ""
	p := b.newProxy(proxyName, pc, configDir, userConfig, dialer)
	b.proxies = append(b.proxies, p)
	go p.start(b.done)
}

func (b *bypassService) newProxy(name string, pc *commonconfig.ProxyConfig, configDir string, userConfig common.UserConfig, dialer dialer.ProxyDialer) *proxy {
	return &proxy{
		ProxyConfig:       pc,
		name:              name,
		proxyRoundTripper: newProxyRoundTripper(name, pc, userConfig, dialer),
		//dfRoundTripper:    b.dfRoundTripper,
		sender:     &sender{},
		toggle:     atomic.NewBool(mrand.Float32() < 0.5),
		userConfig: userConfig,
	}
}

// Reset resets the bypass service by stopping all existing bypass proxy goroutines if
// bypassService is still running. It returns true if bypassService was reset successfully.
func (b *bypassService) Reset() bool {
	if !b.running.Load() {
		return false
	}

	close(b.done)

	b.mxProxies.Lock()
	b.proxies = make([]*proxy, 0)
	b.done = make(chan struct{})
	b.mxProxies.Unlock()

	return true
}

func (b *bypassService) Stop() {
	if b.running.CompareAndSwap(true, false) {
		close(b.done)
	}
}

type proxy struct {
	*commonconfig.ProxyConfig
	name string
	//dfRoundTripper    http.RoundTripper
	proxyRoundTripper http.RoundTripper
	sender            *sender
	toggle            *atomic.Bool
	userConfig        common.UserConfig
}

func (p *proxy) start(done <-chan struct{}) {
	logger.Debugf("Starting bypass for proxy %v", p.name)
	/*
		fn := func() int64 {
			wait, _ := p.sendToBypass()
			return wait
		}
		callRandomly("bypass", fn, bypassSendInterval, done)
	*/
}

/*
func (p *proxy) sendToBypass() (int64, error) {
	op := ops.Begin("bypass_dial")
	defer op.End()

	// We alternate between domain fronting and proxying to ensure that, in aggregate, we
	// send both equally. We avoid sending both a domain fronted and a proxied request
	// in rapid succession to avoid the blocking detection itself being a signal.
	var (
		rt       http.RoundTripper
		endpoint string
		fronted  = p.toggle.Toggle()
	)
	if fronted {
		logger.Debug("bypass: Using domain fronting")
		//rt = p.dfRoundTripper
		endpoint = dfEndpoint
	} else {
		logger.Debug("bypass: Using proxy directly")
		rt = p.proxyRoundTripper
		endpoint = proxyEndpoint
	}

	op.Set("fronted", fronted)

	logger.Debugf("bypass: Sending traffic for bypass server: %v", p.name)
	req, err := p.newRequest(endpoint)
	if err != nil {
		op.FailIf(err)
		return 0, err
	}

	resp, sleep, err := p.sender.post(req, rt)
	if err != nil || resp == nil {
		err = logger.Errorf("bypass: Unable to post chained server info: %v", err)
		op.FailIf(err)
		return 0, err
	}

	if resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return sleep, nil
}
*/

func (p *proxy) newRequest(endpoint string) (*http.Request, error) {
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

	bypassBuf, err := proto.Marshal(bypassRequest)
	if err != nil {
		logger.Errorf("bypass: Unable to marshal chained server info: %v", err)
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bypassBuf))
	if err != nil {
		return nil, fmt.Errorf("unable to create request for %s: %w", endpoint, err)
	}

	common.AddCommonHeaders(p.userConfig, req)
	req.Header.Set("Content-Type", "application/x-protobuf")
	// Prevents intermediate nodes (domain-fronters) from caching the content
	req.Header.Set("Cache-Control", "no-cache")
	return req, nil
}

func newProxyRoundTripper(
	name string,
	info *commonconfig.ProxyConfig,
	userConfig common.UserConfig,
	d dialer.ProxyDialer,
) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		logger.Debugf("bypass: Dialing chained server at: %s", addr)
		pc, _, err := d.DialContext(ctx, dialer.NetworkConnect, addr)
		if err != nil {
			logger.Errorf("bypass: Unable to dial chained server: %v", err)
		} else {
			logger.Debug("bypass: Successfully dialed chained server")
		}

		return pc, err
	}

	return transport
}
