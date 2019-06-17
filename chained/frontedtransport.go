package chained

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/getlantern/eventual"
	"github.com/getlantern/fronted"
	tls "github.com/refraction-networking/utls"
)

const (
	cloudfrontID = "cloudfront"
)

var (
	// special contexts for wss
	frontingContexts = map[string]*fronted.FrontingContext{
		cloudfrontID: fronted.NewFrontingContext(cloudfrontID),
	}
)

type frontedTransport struct {
	rt eventual.Value
}

func (ft *frontedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt, _ := ft.rt.Get(eventual.Forever)
	return rt.(http.RoundTripper).RoundTrip(req)
}

func ConfigureFronting(pool *x509.CertPool, providers map[string]*fronted.Provider, cacheFolder string) {
	// cloudfront only for wss.
	pid := cloudfrontID
	p := providers[pid]
	if p != nil {
		p = fronted.NewProvider(p.HostAliases, p.TestURL, p.Masquerades, p.Validator, []string{"*.cloudfront.net"})
		ponly := map[string]*fronted.Provider{pid: p}
		frontingContexts[pid].ConfigureWithHello(pool, ponly, pid, filepath.Join(cacheFolder, fmt.Sprintf("masquerade_cache.%s", pid)), tls.HelloChrome_Auto)
	}
}

func GetFrontingContext(id string) *fronted.FrontingContext {
	return frontingContexts[id]
}
