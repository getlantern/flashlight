package bypass

// bypass periodically sends traffic to the bypass blocking detection server. The server uses the ratio
// between domain fronted and proxied traffic to determine if proxies are blocked. The client randomizes
// the intervals between calls to the server and also randomizes the length of requests.
import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"

	mrand "math/rand"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
	"go.uber.org/atomic"
)

var log = golog.LoggerFor("bypass")

type bypass struct {
	infos     map[string]*chained.ChainedServerInfo
	proxies   []*proxy
	mxProxies sync.Mutex
}

// Starts client access to the bypass server. The client periodically sends traffic to the server both via
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
		info.Cert = "" // Just save a little space by not sending the cert.
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
}

func (p *proxy) start() {
	log.Debugf("Starting bypass for proxy %v", p.name)
	p.callRandomly(func() {
		p.sendToBypass()
	})
}

func (p *proxy) sendToBypass() {
	req, err := p.newRequest()
	if err != nil {
		log.Errorf("Unable to create request: %v", err)
		return
	}

	// We alternate between domain fronting and proxying to ensure that, in aggregate, we
	// send both equally. We avoid sending both a domain fronted and a proxied request
	// in rapid succession to avoid the blocking detection itself being a signal.
	var rt http.RoundTripper
	if p.toggle.Toggle() {
		rt = p.proxyRoundTripper
	} else {
		rt = p.dfRoundTripper
	}

	log.Debugf("Sending traffic to bypass server: %v", p.name)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		log.Errorf("Unable to post chained server info: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Errorf("Unexpected response code: %v", resp.Status)
	}
	io.ReadAll(resp.Body)
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
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		pc, _, err := dialer.DialContext(ctx, network, addr)
		return pc, err
	}
	return transport
}

type rt func(*http.Request) (*http.Response, error)

func (rt rt) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

func (p *proxy) newRequest() (*http.Request, error) {
	// We include a random length string here to make it harder for censors to identify lantern based on
	// consistent packet lengths.
	p.randString = randomizedString()

	// Just posting all the info about the server allows us to control these fields fully on the server
	// side.
	infoJson, err := json.Marshal(p)
	if err != nil {
		log.Errorf("Unable to marshal chained server info: %v", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://bypass.iantem.io/v1/", bytes.NewBuffer(infoJson))
	if err != nil {
		log.Errorf("Unable to create request: %v", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (p *proxy) stop() {
	p.done <- true
}

// callRandomly calls the given function at a random interval between 2 and 7 minutes.
func (p *proxy) callRandomly(f func()) {
	for {
		select {
		case <-p.done:
			return
		case <-time.After(120 + time.Duration(mrand.Intn(60*5))*time.Second):
			f()
		}
	}
}

func randomizedString() string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	size, err := rand.Int(rand.Reader, big.NewInt(300))
	if err != nil {
		return ""
	}

	bytes := make([]byte, size.Int64())
	for i := range bytes {
		bytes[i] = charset[mrand.Intn(len(charset))]
	}
	return string(bytes)
}
