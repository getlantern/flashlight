package bypass

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"sync"
	"time"

	mrand "math/rand"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("bypass")

type bypass struct {
	client    *http.Client
	infos     map[string]*chained.ChainedServerInfo
	proxies   []*proxy
	mxProxies sync.Mutex
}

type proxy struct {
	*chained.ChainedServerInfo
	name       string
	done       chan bool
	randString string
}

func (b *bypass) OnProxies(infos map[string]*chained.ChainedServerInfo) {
	b.mxProxies.Lock()
	defer b.mxProxies.Unlock()
	b.reset()
	for k, v := range infos {
		p := b.newProxy(k, v)
		b.proxies = append(b.proxies, p)
		go p.start(b.client)
	}
}

func (b *bypass) newProxy(name string, info *chained.ChainedServerInfo) *proxy {
	return &proxy{
		ChainedServerInfo: info,
		name:              name,
		done:              make(chan bool),
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

func (b *bypass) reset() {
	for _, v := range b.proxies {
		v.stop()
	}
	b.proxies = make([]*proxy, 0)
}

// Starts client access to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start(listen func(flashlight.ProxyListener)) {
	StartWith(http.DefaultClient, listen)
}

func StartWith(client *http.Client, listen func(flashlight.ProxyListener)) {
	b := &bypass{client: client, proxies: make([]*proxy, 0)}
	listen(b)
}

func (p *proxy) start(client *http.Client) {
	p.callRandomly(func() {
		// We include a random length string here to make it harder for censors to identify lantern based on
		// consistent packet lengths.
		p.randString = randomizedString()

		// Just posting all the info about the server allows us to control these fields fully on the server
		// side.
		infoJson, err := json.Marshal(p)
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
		case <-time.After(90 + time.Duration(mrand.Intn(60))*time.Second):
			f()
		}
	}
}
