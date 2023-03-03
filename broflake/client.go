package broflake

import (
	"context"
	"net"
	"net/http"

	"github.com/getlantern/broflake/clientcore"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.broflake")

	client               clientcore.ReliableStreamLayer
	broflakeRoundTripper http.RoundTripper
	ready                = make(chan struct{})
)

// Dials a connection to a broflake egress server
func DialContext(ctx context.Context) (net.Conn, error) {
	select {
	case <-ready:
		return client.DialContext(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type roundTripper struct{}

func (*roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	select {
	case <-ready:
		return broflakeRoundTripper.RoundTrip(req)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Creates a new http.RoundTripper that uses broflake to proxy http requests.
//
// Calls to the RoundTripper will block until
// broflake has been initialized and has provided a dialer.
func NewRoundTripper() http.RoundTripper {
	return &roundTripper{}
}

type Options struct {
	BroflakeOptions  *clientcore.BroflakeOptions
	WebRTCOptions    *clientcore.WebRTCOptions
	QUICLayerOptions *clientcore.QUICLayerOptions
}

// Initializes and starts broflake in a configuration suitable
// for a flashlight censored peer.
func InitAndStartBroflakeCensoredPeer(options *Options) error {
	bfconn, _, err := clientcore.NewBroflake(options.BroflakeOptions, options.WebRTCOptions, nil)
	if err != nil {
		return err
	}
	ql, err := clientcore.NewQUICLayer(bfconn, options.QUICLayerOptions)
	if err != nil {
		return err
	}
	client = ql
	broflakeRoundTripper = clientcore.CreateHTTPTransport(ql)
	close(ready)

	return nil
}
