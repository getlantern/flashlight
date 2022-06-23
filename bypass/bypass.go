package bypass

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("bypass")

type bypass struct {
	client    *http.Client
	infos     map[string]*chained.ChainedServerInfo
	proxies   map[string]*proxy
	mxProxies sync.Mutex
}

type proxy struct {
	info *chained.ChainedServerInfo
	done chan bool
}

func (b *bypass) OnProxies(infos map[string]*chained.ChainedServerInfo) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	b.reset()
	for k, v := range infos {
		p := &proxy{info: v, done: make(chan bool)}
		b.proxies[k] = p
		go p.start(b.client)
	}
}

func (b *bypass) reset() {
	for _, v := range b.proxies {
		v.stop()
	}
	b.proxies = make(map[string]*proxy)
}

// Starts client access to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start(listen func(flashlight.ProxyListener)) {
	StartWith(http.DefaultClient, listen)
}

func StartWith(client *http.Client, listen func(flashlight.ProxyListener)) {
	b := &bypass{client: client, proxies: make(map[string]*proxy)}
	listen(b)
}

func (p *proxy) start(client *http.Client) {
	p.callRandomly(func() {
		// Just posting all the info about the server allows us to control these fields fully on the server
		// side.
		infoJson, err := json.Marshal(p.info)
		if err != nil {
			log.Errorf("Unable to marshal chained server info: %v", err)
			return
		}

		// TODO: Randomize length of the post.
		client.Post("https://bypass.iantem.io/v1/", "application/json",
			bytes.NewBuffer(infoJson))
	})
}

func (p *proxy) stop() {
	p.done <- true
}

func (p *proxy) callRandomly(f func()) {
	for {
		select {
		case <-p.done:
			return
		case <-time.After(90 + time.Duration(rand.Intn(60))*time.Second):
			f()
		}
	}
}
