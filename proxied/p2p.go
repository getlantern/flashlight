package proxied

import (
	"net/http"

	"github.com/getlantern/libp2p/p2p"
)

// P2P returns an http.RoundTripper capable of proxying requests through peers.
func P2P(p2pCtx *p2p.CensoredP2pCtx) http.RoundTripper {
	// XXX <01-02-22, soltzen> This function doesn't do much. It's here mainly
	// since the proxied package takes care of all proxying behaviour and it's
	// best to keep things centrailized
	return p2pCtx
}
