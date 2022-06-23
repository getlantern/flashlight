package bypass

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	mrand "math/rand"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("bypass")

type bypass struct {
	infos     map[string]*chained.ChainedServerInfo
	proxies   []*proxy
	mxProxies sync.Mutex
}

// Starts client access to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start(listen func(func(map[string]*chained.ChainedServerInfo))) func() {
	b := &bypass{proxies: make([]*proxy, 0)}
	listen(b.OnProxies)
	return b.reset
}

func (b *bypass) OnProxies(infos map[string]*chained.ChainedServerInfo) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	b.reset()
	for k, v := range infos {
		p := b.newProxy(k, v)
		b.proxies = append(b.proxies, p)
		go p.start()
	}
}

func (b *bypass) newProxy(name string, info *chained.ChainedServerInfo) *proxy {
	return &proxy{
		ChainedServerInfo: info,
		name:              name,
		done:              make(chan bool),
		rt:                proxied.Parallel(),
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
	name       string
	done       chan bool
	randString string
	rt         http.RoundTripper
}

func (p *proxy) start() {
	p.callRandomly(func() {
		req, err := p.newRequest()
		if err != nil {
			log.Errorf("Unable to create reques: %v", err)
			return
		}
		resp, err := p.rt.RoundTrip(req)
		if err != nil {
			log.Errorf("Unable to post chained server info: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Errorf("Unexpected response code: %v", resp.Status)
		}
		io.ReadAll(resp.Body)
	})
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

func (p *proxy) callRandomly(f func()) {
	for {
		select {
		case <-p.done:
			return
		case <-time.After(90 + time.Duration(mrand.Intn(60))*time.Second):
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
