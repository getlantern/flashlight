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
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
	"github.com/getlantern/lantern-cloud/cmd/api/apipb"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"
)

var (
	log      = golog.LoggerFor("bypass")
	endpoint = "https://iantem.io/bypass/v1/proxy"
)

type bypass struct {
	infos     map[string]*chained.ChainedServerInfo
	proxies   []*proxy
	mxProxies sync.Mutex
}

// Start sends periodic traffic to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start(listen func(func(map[string]*chained.ChainedServerInfo)), configDir string, userConfig common.UserConfig) func() {
	mrand.Seed(time.Now().UnixNano())
	b := &bypass{
		infos:   make(map[string]*chained.ChainedServerInfo),
		proxies: make([]*proxy, 0),
	}
	listen(func(infos map[string]*chained.ChainedServerInfo) {
		b.OnProxies(infos, configDir, userConfig)
	})
	return b.reset
}

func (b *bypass) OnProxies(infos map[string]*chained.ChainedServerInfo, configDir string, userConfig common.UserConfig) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	b.reset()
	for k, v := range infos {
		info := new(chained.ChainedServerInfo)
		*info = *v
		// Set the name in the info since we know it here.
		info.Name = k
		p := b.newProxy(k, info, configDir, userConfig)
		b.proxies = append(b.proxies, p)
		go p.start()
	}
}

func (b *bypass) newProxy(name string, info *chained.ChainedServerInfo, configDir string, userConfig common.UserConfig) *proxy {
	return &proxy{
		ChainedServerInfo: info,
		name:              name,
		done:              make(chan bool),
		toggle:            atomic.NewBool(mrand.Float32() < 0.5),
		dfRoundTripper:    proxied.Fronted(),
		userConfig:        userConfig,
		proxyRoundTripper: proxyRoundTripper(name, info, configDir, userConfig),
	}
}

func (b *bypass) reset() {
	for _, v := range b.proxies {
		v.stop()
	}
	b.proxies = make([]*proxy, 0)
}

type proxy struct {
	*chained.ChainedServerInfo
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
	req, err := p.newRequest(p.userConfig)
	if err != nil {
		log.Errorf("Unable to create request: %v", err)
		return 0
	}

	// We alternate between domain fronting and proxying to ensure that, in aggregate, we
	// send both equally. We avoid sending both a domain fronted and a proxied request
	// in rapid succession to avoid the blocking detection itself being a signal.
	var rt http.RoundTripper
	if p.toggle.Toggle() {
		log.Debug("Using proxy directly")
		rt = p.proxyRoundTripper
	} else {
		rt = p.dfRoundTripper
		log.Debug("Using domain fronting")
	}

	log.Debugf("Sending traffic for bypass server: %v", p.name)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		log.Errorf("Unable to post chained server info: %v", err)
		return 0
	}
	defer func() {
		if closeerr := resp.Body.Close(); closeerr != nil {
			log.Errorf("Error closing response body: %v", closeerr)
		}
	}()

	io.Copy(io.Discard, resp.Body)

	var sleepTime int64
	sleepVal := resp.Header.Get(common.SleepHeader)
	if sleepVal != "" {
		sleepTime, err = strconv.ParseInt(sleepVal, 10, 64)
		if err != nil {
			log.Errorf("Could not parse sleep val: %v", err)
		}
	}
	if resp.StatusCode != http.StatusOK {
		log.Errorf("Unexpected response code: %v", resp.Status)
		// If we don't get a 200, we'll revert to the default sleep time.
		return -1
	} else {
		log.Debugf("Successfully sent traffic for bypass server: %v", p.name)
	}
	return sleepTime
}

func proxyRoundTripper(name string, info *chained.ChainedServerInfo, configDir string, userConfig common.UserConfig) http.RoundTripper {
	dialer, err := chained.CreateDialer(configDir, name, info, userConfig)
	if err != nil {
		log.Errorf("Unable to create dialer: %v", err)
		return rt(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
			}, nil
		})
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		pc, _, err := dialer.DialContext(ctx, balancer.NetworkConnect, addr)
		return pc, err
	}
	return transport
}

type rt func(*http.Request) (*http.Response, error)

func (rt rt) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

func (p *proxy) newRequest(userConfig common.UserConfig) (*http.Request, error) {
	// Just posting all the info about the server allows us to control these fields fully on the server
	// side.
	info := new(chained.ChainedServerInfo)
	*info = *p.ChainedServerInfo
	info.Cert = "" // Just save a little space by not sending the cert.

	commonInfo := apipb.ProxyConfig(*info)
	infopb, err := proto.Marshal(&commonInfo)
	if err != nil {
		log.Errorf("Unable to marshal chained server info: %v", err)
		return nil, err
	}

	if err != nil {
		log.Errorf("Unable to write chained server info: %v", err)
		return nil, err
	}

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
	log.Debugf("Sending request: %#v", req)

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
		base := 60
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
