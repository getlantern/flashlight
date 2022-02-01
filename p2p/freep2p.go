package p2p

import (
	"context"
	"encoding/hex"
	"net"
	"net/http"
	"sync"

	"github.com/anacrolix/dht/v2"
	"github.com/elazarl/goproxy"
)

type FreeP2pCtx struct {
	dhtServer   *dht.Server
	infohashes  [][20]byte
	addr        *net.TCPAddr
	p2pFuncs    ReplicaP2pFunctions
	proxyServer *http.Server
	closeOnce   sync.Once
	closeChan   chan struct{}
}

func NewFreeP2pCtx(
	peerInfoHashes []string,
	p2pFuncs ReplicaP2pFunctions,
) (*FreeP2pCtx, error) {
	cfg := dht.NewDefaultServerConfig()
	cfg.NoSecurity = false
	s, err := dht.NewServer(cfg)
	if err != nil {
		return nil, log.Errorf("%v", err)
	}

	// Clean infohashes
	ihs := [][20]byte{}
	for _, v := range peerInfoHashes {
		var ih [20]byte
		h, err := hex.DecodeString(v)
		if err != nil {
			return nil, log.Errorf("%v", err)
		}
		copy(ih[:], h)
		ihs = append(ihs, ih)
	}

	return &FreeP2pCtx{
		dhtServer:  s,
		infohashes: ihs,
		p2pFuncs:   p2pFuncs,
		closeChan:  make(chan struct{}),
	}, nil
}

func (p2pCtx *FreeP2pCtx) StartConnectProxy(errChan chan error) (int, error) {
	log.Debugf("Free peer: Initializing CONNECT proxy...")
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, log.Errorf("starting p2p CONNECT proxy: %v", err)
	}
	proxy := goproxy.NewProxyHttpServer()
	// proxy.Verbose = true
	go func() {
		log.Debugf("Free peer: Starting p2p CONNECT proxy on %s", ln.Addr())
		p2pCtx.proxyServer = &http.Server{Addr: ln.Addr().String(), Handler: proxy}
		err = p2pCtx.proxyServer.Serve(ln)
		if err != nil && err != http.ErrServerClosed && errChan != nil {
			errChan <- log.Errorf("P2p CONNECT proxy server failed: %v", err)
		}
		close(errChan)
	}()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (p2pCtx *FreeP2pCtx) Announce(port int) error {
	log.Debugf("Free peer: Announcing %+v to port %v", p2pCtx.infohashes, port)
	return p2pCtx.p2pFuncs.Announce(
		[]*dht.Server{p2pCtx.dhtServer},
		p2pCtx.infohashes,
		port)
}

// Close shutsdown this peer's resources.
func (p2pCtx *FreeP2pCtx) Close(ctx context.Context) {
	p2pCtx.closeOnce.Do(func() {
		log.Debugf("Free peer: Closing...")
		if p2pCtx.dhtServer != nil {
			p2pCtx.dhtServer.Close()
		}
		if p2pCtx.proxyServer != nil {
			err := p2pCtx.proxyServer.Shutdown(ctx)
			if err != nil {
				log.Debugf("Error while closing proxy server: %v", err)
			}
		}
		close(p2pCtx.closeChan)
	})
}

func (p2pCtx *FreeP2pCtx) IsClosed() <-chan struct{} {
	return p2pCtx.closeChan
}
