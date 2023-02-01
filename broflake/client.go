package broflake

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/getlantern/broflake/clientcore"
	bfcommon "github.com/getlantern/broflake/common"
	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/lucas-clemente/quic-go"
)

const (
	cTableSize  = 5
	pTableSize  = 5
	busBufferSz = 2048
)

var (
	log = golog.LoggerFor("flashlight.broflake")

	dialer = eventual.NewValue()
	ui     = clientcore.UIImpl{}
	bus    = clientcore.NewIpcObserver(
		busBufferSz,
		clientcore.UpstreamUIHandler(ui),
		clientcore.DownstreamUIHandler(ui),
	)
)

type dialerFn func(network, addr string) (net.Conn, error)

// Dials a connection to a broflake egress server
//
// Calls to Dial will block until
// broflake has been initialized and has provided a dialer.
func Dial(network string, addr string) (net.Conn, error) {
	d, _ := dialer.Get(eventual.Forever)
	return d.(dialerFn)(network, addr)
}

func newQUICDialerFn(pconn net.PacketConn, tlsConfig *tls.Config) dialerFn {
	client := &client{pconn: pconn, tlsConfig: tlsConfig}
	return func(network, addr string) (net.Conn, error) {
		return client.DialContext(context.Background())
	}
}

type client struct {
	pconn     net.PacketConn
	tlsConfig *tls.Config
	session   quic.Connection
	mx        sync.Mutex
}

func (c *client) DialContext(ctx context.Context) (net.Conn, error) {
	session, err := c.getOrCreateSession(ctx)
	if err != nil {
		log.Debugf("failed to create quic session over broflake pconn: %s", err)
		return nil, fmt.Errorf("connecting broflake session: %w", err)
	}
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		// TODO is there a more limited set of errors that cause session to be resestablished?
		c.clearSession(err.Error())
		return nil, fmt.Errorf("establishing broflake stream: %w", err)
	}
	return &bfcommon.QUICStreamNetConn{Stream: stream}, nil
}

func (c *client) getOrCreateSession(ctx context.Context) (quic.Connection, error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	if c.session == nil {
		session, err := quic.DialContext(
			ctx,
			c.pconn,
			bfcommon.DebugAddr("broflake address placeholder"),
			"",
			c.tlsConfig,
			&bfcommon.QUICCfg)

		if err != nil {
			return nil, err
		}
		c.session = session
	}
	return c.session, nil
}

func (c *client) clearSession(reason string) {
	c.mx.Lock()
	s := c.session
	c.session = nil
	c.mx.Unlock()
	if s != nil {
		log.Debugf("Closing broflake session (%v)", reason)
		s.CloseWithError(0, "")
	}
}

// Creates a new http.RoundTripper that uses broflake to proxy http requests.
//
// Calls to the RoundTripper will block until
// broflake has been initialized and has provided a dialer.
func NewRoundTripper() *http.Transport {
	return &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse("http://i.do.nothing")
		},
		Dial: Dial,
	}
}

type Options struct {
	WebRTCOptions            *clientcore.WebRTCOptions
	EgressServerName         string
	EgressInsecureSkipVerify bool
}

// Initializes and starts broflake in a configuration suitable
// for a flashlight censored peer.
func InitAndStartBroflakeCensoredPeer(options *Options) error {
	var wgReady sync.WaitGroup
	bfconn, producerUserStream := clientcore.NewProducerUserStream(&wgReady)
	cTable := clientcore.NewWorkerTable([]clientcore.WorkerFSM{*producerUserStream})
	cRouter := clientcore.NewConsumerRouter(bus.Downstream, cTable)
	var pfsms []clientcore.WorkerFSM
	for i := 0; i < pTableSize; i++ {
		pfsms = append(pfsms, *clientcore.NewConsumerWebRTC(options.WebRTCOptions, &wgReady))
	}
	pTable := clientcore.NewWorkerTable(pfsms)
	pRouter := clientcore.NewProducerSerialRouter(bus.Upstream, pTable, cTable.Size())
	broflake := clientcore.NewBroflake(cTable, pTable, &ui, &wgReady)
	ui.Init(broflake)
	bus.Start()
	cRouter.Init()
	pRouter.Init()
	tlsConfig := &tls.Config{
		ServerName:         options.EgressServerName,
		InsecureSkipVerify: options.EgressInsecureSkipVerify,
		NextProtos:         []string{"broflake"},
	}
	dialer.Set(newQUICDialerFn(bfconn, tlsConfig))
	ui.OnReady()
	ui.OnStartup()
	return nil
}
