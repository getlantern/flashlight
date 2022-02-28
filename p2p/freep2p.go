package p2p

import (
	"context"
	"encoding/hex"
	"net"
	"strconv"
	"sync"

	"github.com/anacrolix/dht/v2"
	"github.com/getlantern/flashlight/quicproxy"
	"github.com/getlantern/flashlight/upnp"
)

type FreeP2pCtx struct {
	UpnpClient         *upnp.Client
	ReverseProxyServer *quicproxy.QuicReverseProxy

	dhtServer  *dht.Server
	infohashes [][20]byte
	addr       *net.TCPAddr
	p2pFuncs   ReplicaP2pFunctions
	closeOnce  sync.Once
	closeChan  chan struct{}
}

func NewFreeP2pCtx(
	peerInfoHashes []string,
	p2pFuncs ReplicaP2pFunctions,
) (*FreeP2pCtx, error) {
	if peerInfoHashes == nil || len(peerInfoHashes) == 0 {
		return nil, log.Errorf("peerInfoHashes is nil or empty")
	}
	if p2pFuncs == nil {
		return nil, log.Errorf("p2pFuncs is nil")
	}

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
		UpnpClient: upnp.New(),
	}, nil
}

func (p2pCtx *FreeP2pCtx) StartReverseProxy(
	port int,
	pemEncodedCert, pemEncodedPrivKey []byte,
	errChan chan error,
	verbose bool) error {
	log.Debugf("Free peer: Initializing CONNECT proxy...")

	p, err := quicproxy.NewReverseProxy(
		":"+strconv.Itoa(port), // addr
		pemEncodedCert,
		pemEncodedPrivKey,
		verbose,
		errChan,
	)
	if err != nil {
		return log.Errorf("while NewReverseProxy %v", err)
	}
	p2pCtx.ReverseProxyServer = p
	return nil
}

func (p2pCtx *FreeP2pCtx) Announce() error {
	if p2pCtx.ReverseProxyServer.Port == 0 {
		return log.Errorf("Reverse proxy port is 0. Cannot proceed")
	}
	log.Debugf("Free peer: Announcing %+v to port %v",
		p2pCtx.infohashes, p2pCtx.ReverseProxyServer.Port)
	return p2pCtx.p2pFuncs.Announce(
		[]*dht.Server{p2pCtx.dhtServer},
		p2pCtx.infohashes,
		p2pCtx.ReverseProxyServer.Port)
}

// Close shutsdown this peer's resources.
func (p2pCtx *FreeP2pCtx) Close(ctx context.Context) {
	p2pCtx.closeOnce.Do(func() {
		log.Debugf("Free peer: Closing...")
		if p2pCtx.dhtServer != nil {
			p2pCtx.dhtServer.Close()
		}
		if p2pCtx.ReverseProxyServer != nil {
			err := p2pCtx.ReverseProxyServer.Shutdown(ctx)
			if err != nil {
				log.Debugf("Error while closing reverse proxy server: %v", err)
			}
		}
		close(p2pCtx.closeChan)
	})
}

func (p2pCtx *FreeP2pCtx) IsClosed() <-chan struct{} {
	return p2pCtx.closeChan
}
