package chained

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/getlantern/golog"
	"github.com/getlantern/lampshade"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
)

const lampshadeTransport = "lampshade"

type lampshadeProxy struct {
	log    golog.Logger
	s      *ChainedServerInfo
	name   string
	proto  string
	dialer lampshade.Dialer
	opts   *lampshade.DialerOpts
}

/*
func newLampshadeProxy2(s *ChainedServerInfo, name, proto string, uc common.UserConfig) (*proxy, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", s.Addr))
	}
	rsaPublicKey, ok := cert.X509().PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("Public key is not an RSA public key!")
	}
	cipherCode := lampshade.Cipher(s.ptSettingInt(fmt.Sprintf("cipher_%v", runtime.GOARCH)))
	if cipherCode == 0 {
		if runtime.GOARCH == "amd64" {
			// On 64-bit Intel, default to AES128_GCM which is hardware accelerated
			cipherCode = lampshade.AES128GCM
		} else {
			// default to ChaCha20Poly1305 which is fast even without hardware acceleration
			cipherCode = lampshade.ChaCha20Poly1305
		}
	}
	windowSize := s.ptSettingInt("windowsize")
	maxPadding := s.ptSettingInt("maxpadding")
	maxStreamsPerConn := uint16(s.ptSettingInt("streams"))
	idleInterval, parseErr := time.ParseDuration(s.ptSetting("idleinterval"))
	if parseErr != nil || idleInterval < 0 {
		// This should be less than the server's IdleTimeout to avoid trying to use
		// a connection that was just idled. The client's IdleTimeout is already set
		// appropriately for this purpose, so use that.
		idleInterval = IdleTimeout
		log.Debugf("%s: defaulted idleinterval to %v", name, idleInterval)
	}
	pingInterval, parseErr := time.ParseDuration(s.ptSetting("pinginterval"))
	if parseErr != nil || pingInterval < 0 {
		pingInterval = 15 * time.Second
		log.Debugf("%s: defaulted pinginterval to %v", name, pingInterval)
	}
	maxLiveConns := s.ptSettingInt("maxliveconns")
	if maxLiveConns <= 0 {
		maxLiveConns = 5
		log.Debugf("%s: defaulted maxliveconns to %v", name, maxLiveConns)
	}
	redialSessionInterval, parseErr := time.ParseDuration(s.ptSetting("redialsessioninterval"))
	if parseErr != nil || redialSessionInterval < 0 {
		redialSessionInterval = 5 * time.Second
		log.Debugf("%s: defaulted redialsessioninterval to %v", name, redialSessionInterval)
	}
	opts := &lampshade.DialerOpts{
		WindowSize:            windowSize,
		MaxPadding:            maxPadding,
		MaxLiveConns:          maxLiveConns,
		MaxStreamsPerConn:     maxStreamsPerConn,
		IdleInterval:          idleInterval,
		PingInterval:          pingInterval,
		RedialSessionInterval: redialSessionInterval,
		Pool:                  buffers.Pool,
		Cipher:                cipherCode,
		ServerPublicKey:       rsaPublicKey,
	}
	lifecycle := newLampshadeLifecycleListener(name)

	lp := &lampshadeProxy{
		name:  name,
		proto: proto,
		s:     s,
		log:   golog.LoggerFor("chained.lampshade"),
		opts:  opts,
		dialer: lampshade.NewDialer(opts, lifecycle, func() (net.Conn, error) {
						// note - we do not wrap the TCP connection with IdleTiming because
			// lampshade cleans up after itself and won't leave excess unused
			// connections hanging around.
			log.Debugf("Dialing lampshade TCP connection to %v", p.Label())
			return p.dialCore(op)(ctx)
		}),
	}

	dialServer := p.reportedDial(s.Addr, lampshadeTransport, proto, func(op *ops.Op) (net.Conn, error) {
		op.Set("ls_win", opts.WindowSize).
			Set("ls_pad", opts.MaxPadding).
			Set("ls_streams", int(opts.MaxStreamsPerConn)).
			Set("ls_cipher", opts.Cipher.String())
		// Note this can either dial a new TCP connection or, in most cases, create a new
		// stream over an existing TCP connection/lampshade session.
		lifecycle := newLampshadeLifecycleListener(name)
		ctx = context.WithValue(ctx, uhk, upstreamHost)
		conn, err := lp.dialer.DialContext(ctx, lifecycle)
		return overheadWrapper(true)(conn, err)
	})

	p, err := newProxy(name, lampshadeTransport, proto, s, uc, s.Trusted, false, dialServer, lp.dialOrigin)

	return p, err
}

func (l *lampshadeProxy) newProxy(uc common.UserConfig) (*proxy, error) {
	l.log.Debugf("Creating lampshade proxy with ptSettings: %#v", l.s)
	p, err := newProxy(l.name, lampshadeTransport, l.proto, l.s, uc, l.s.Trusted, false, l.dialServer, l.dialOrigin)
	return p, err
}

func (l *lampshadeProxy) dialServer(ctx context.Context, proxyName, upstreamHost string, p *proxy) (net.Conn, error) {
	return p.reportedDial(l.s.Addr, lampshadeTransport, l.proto, func(op *ops.Op) (net.Conn, error) {
		op.Set("ls_win", l.opts.WindowSize).
			Set("ls_pad", l.opts.MaxPadding).
			Set("ls_streams", int(l.opts.MaxStreamsPerConn)).
			Set("ls_cipher", l.opts.Cipher.String())
		// Note this can either dial a new TCP connection or, in most cases, create a new
		// stream over an existing TCP connection/lampshade session.
		lifecycle := newLampshadeLifecycleListener(proxyName)
		ctx = context.WithValue(ctx, uhk, upstreamHost)
		conn, err := l.dialer.DialContext(ctx, lifecycle)
		return overheadWrapper(true)(conn, err)
	})
}

func (l *lampshadeProxy) dialOrigin(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error) {
	return defaultDialOrigin(op, ctx, p, network, addr)
}
*/

func newLampshadeLifecycleListener(proxyName string) lampshade.ClientLifecycleListener {
	return &lampshadeLifecycleListener{
		proxyName: proxyName,
	}
}

func newLampshadeStreamListener(span opentracing.Span, ll *lampshadeLifecycleListener) lampshade.StreamLifecycleListener {
	return &lampshadeStreamListener{
		span: span,
		ll:   ll,
	}
}

type upstreamHostKey string

var uhk = upstreamHostKey("uhk")

type lampshadeLifecycleListener struct {
	proxyName    string
	dialSpan     atomic.Value
	sessionSpan  atomic.Value
	totalRead    int64
	totalWritten int64
}

func (ll *lampshadeLifecycleListener) OnSessionInit(ctx context.Context) context.Context {
	// We start with this somewhat alarming name because it will be rewritten upon successful connection.
	span, ctx := opentracing.StartSpanFromContext(ctx, "lampshade-failed-TCP")
	//ll.sessionSpan.Store(opentracing.StartSpan("lampshade-failed-TCP"))
	//ll.sessionSpan.Load().(opentracing.Span).SetTag("proxy", ll.proxyName)

	ll.sessionSpan.Store(span)
	return ctx
	//return opentracing.ContextWithSpan(ctx, ll.sessionSpan.Load().(opentracing.Span))
}

func (ll *lampshadeLifecycleListener) OnRedialSessionInterval(context.Context) {
	/*
		ll.sessionSpan.Load().(opentracing.Span).SetTag("session-redial", "true")
		ll.sessionSpan.Load().(opentracing.Span).LogFields(otlog.Bool("session-redial", true))
	*/
}
func (ll *lampshadeLifecycleListener) OnTCPStart(sessionContext context.Context) {
	/*
		opts := []opentracing.StartSpanOption{opentracing.ChildOf(ll.sessionSpan.Load().(opentracing.Span).Context())}
		ll.dialSpan.Store(opentracing.GlobalTracer().StartSpan("lampshade-tcp-dial", opts...))
	*/
}
func (ll *lampshadeLifecycleListener) OnTCPConnectionError(err error) {
	ll.dialSpan.Load().(opentracing.Span).SetTag("error", err.Error())
}
func (ll *lampshadeLifecycleListener) OnTCPEstablished(conn net.Conn) {
	/*
		local := conn.LocalAddr().(*net.TCPAddr)
		ll.sessionSpan.Load().(opentracing.Span).SetTag("proto", "lampshade")
		ll.sessionSpan.Load().(opentracing.Span).SetTag("host", conn.RemoteAddr().String())
		ll.sessionSpan.Load().(opentracing.Span).SetTag("clientport", local.Port)
		ll.sessionSpan.Load().(opentracing.Span).SetOperationName(fmt.Sprintf("%v->%v", local.Port, ll.proxyName))

		ll.dialSpan.Load().(opentracing.Span).Finish()

		// We call finish here because the sessions's Close method is often not called, and
		// without a finished parent span child spans seem to often be left orphaned.
		ll.sessionSpan.Load().(opentracing.Span).Finish()
	*/
}

func (ll *lampshadeLifecycleListener) OnTCPClosed()                        {}
func (ll *lampshadeLifecycleListener) OnClientInitWritten(context.Context) {}
func (ll *lampshadeLifecycleListener) OnSessionError(readErr error, writeErr error) {
	/*
		if readErr != nil {
			ll.sessionSpan.Load().(opentracing.Span).LogFields(otlog.Error(readErr))
			ll.sessionSpan.Load().(opentracing.Span).SetTag("error", "session")
			ll.sessionSpan.Load().(opentracing.Span).SetTag("fullerror", readErr.Error())
		}
		if writeErr != nil {
			ll.sessionSpan.Load().(opentracing.Span).LogFields(otlog.Error(writeErr))
			ll.sessionSpan.Load().(opentracing.Span).SetTag("error", "session")
			ll.sessionSpan.Load().(opentracing.Span).SetTag("fullerror", writeErr.Error())
		}

		ll.sessionSpan.Load().(opentracing.Span).Finish()
	*/
}

func (ll *lampshadeLifecycleListener) OnStreamInit(ctx context.Context, id uint16) lampshade.StreamLifecycleListener {
	opts := make([]opentracing.StartSpanOption, 0)
	var span opentracing.Span
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
		uh := ctx.Value(uhk)
		span = opentracing.GlobalTracer().StartSpan(fmt.Sprintf("stream-%v-%v", id, uh), opts...)
	} else {
		noop := opentracing.NoopTracer{}
		span = noop.StartSpan("noop")
	}
	return newLampshadeStreamListener(span, ll)
}
func (ls *lampshadeStreamListener) OnStreamRead(num int) {
	/*
		ls.span.LogFields(otlog.Int("r", num))
		ls.rm.Lock()
		atomic.AddInt64(&ls.ll.totalRead, int64(num))
		ls.ll.sessionSpan.Load().(opentracing.Span).SetTag("read", ls.ll.totalRead)
		ls.rm.Unlock()
	*/
}
func (ls *lampshadeStreamListener) OnStreamWrite(num int) {
	/*
		ls.span.LogFields(otlog.Int("w", num))
		ls.wm.Lock()
		atomic.AddInt64(&ls.ll.totalWritten, int64(num))
		ls.ll.sessionSpan.Load().(opentracing.Span).SetTag("written", ls.ll.totalWritten)
		ls.wm.Unlock()
	*/
}
func (ls *lampshadeStreamListener) OnStreamClose() {
	ls.span.LogFields(otlog.String("closed", "true"))
	ls.span.Finish()
}

type lampshadeStreamListener struct {
	span opentracing.Span
	ll   *lampshadeLifecycleListener
	wm   sync.RWMutex
	rm   sync.RWMutex
}
