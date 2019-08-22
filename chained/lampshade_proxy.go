package chained

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/golog"
	"github.com/getlantern/keyman"
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

func newLampshadeProxy2(s *ChainedServerInfo, name, proto string) (*lampshadeProxy, error) {
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
	return &lampshadeProxy{
		name:   name,
		proto:  proto,
		s:      s,
		log:    golog.LoggerFor("chained.lampshade"),
		opts:   opts,
		dialer: lampshade.NewDialer(opts),
	}, nil
}

func (l *lampshadeProxy) newProxy(uc common.UserConfig) (*proxy, error) {
	l.log.Debugf("Creating lampshade proxy with ptSettings: %#v", l.s)
	return newProxy(l.name, lampshadeTransport, l.proto, l.s, uc, l.s.Trusted, false, l.dialServer, l.dialOrigin)
}

func (l *lampshadeProxy) dialServer(ctx context.Context, proxyName, upstreamHost string, p *proxy) (net.Conn, error) {
	return p.reportedDial(l.s.Addr, lampshadeTransport, l.proto, func(op *ops.Op) (net.Conn, error) {
		op.Set("ls_win", l.opts.WindowSize).
			Set("ls_pad", l.opts.MaxPadding).
			Set("ls_streams", int(l.opts.MaxStreamsPerConn)).
			Set("ls_cipher", l.opts.Cipher.String())
		// Note this can either dial a new TCP connection or, in most cases, create a new
		// stream over an existing TCP connection/lampshade session.
		lifecycle := newLampshadeLifecycleListener(proxyName, upstreamHost)
		conn, err := l.dialer.DialContext(ctx, lifecycle, func() (net.Conn, error) {
			// note - we do not wrap the TCP connection with IdleTiming because
			// lampshade cleans up after itself and won't leave excess unused
			// connections hanging around.
			l.log.Debugf("Dialing lampshade TCP connection to %v", p.Label())
			return p.dialCore(op)(ctx)
		})
		//return overheadWrapper(true)(conn, err)
		return conn, err
	})
}

func (l *lampshadeProxy) dialOrigin(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error) {
	return defaultDialOrigin(op, ctx, p, network, addr)
}

func newLampshadeLifecycleListener(proxyName, upstreamHost string) lampshade.LifecycleListener {
	return &lampshadeLifecycleListener{
		proxyName:    proxyName,
		upstreamHost: upstreamHost,
	}
}

func newLampshadeStreamListener(span opentracing.Span) lampshade.StreamLifecycleListener {
	return &lampshadeStreamListener{
		span: span,
	}
}

type lampshadeLifecycleListener struct {
	proxyName    string
	upstreamHost string
	dialSpan     opentracing.Span
	sessionSpan  opentracing.Span
}

func (ll *lampshadeLifecycleListener) OnSessionInit(ctx context.Context) context.Context {
	ll.sessionSpan = opentracing.StartSpan("lampshade-failed-TCP")
	return opentracing.ContextWithSpan(ctx, ll.sessionSpan)
}
func (ll *lampshadeLifecycleListener) OnTCPConnReceived()             {}
func (ll *lampshadeLifecycleListener) OnReadClientInitError(string)   {}
func (ll *lampshadeLifecycleListener) OnDecodeClientInitError(string) {}
func (ll *lampshadeLifecycleListener) OnTCPStart(sessionContext context.Context) {
	opts := []opentracing.StartSpanOption{opentracing.ChildOf(ll.sessionSpan.Context())}
	ll.dialSpan = opentracing.GlobalTracer().StartSpan("lampshade-dial-init", opts...)
}
func (ll *lampshadeLifecycleListener) OnTCPConnectionError(err error) {
	ll.dialSpan.SetTag("error", err.Error())
}
func (ll *lampshadeLifecycleListener) OnTCPEstablished(conn net.Conn) {
	local := conn.LocalAddr().(*net.TCPAddr)
	ll.sessionSpan.SetTag("proto", "lampshade")
	ll.sessionSpan.SetTag("host", conn.RemoteAddr().String())
	ll.sessionSpan.SetTag("clientport", local.Port)
	ll.sessionSpan.SetOperationName(fmt.Sprintf("%s->%v", ll.proxyName, local.Port))

	ll.dialSpan.Finish()

	// We call finish here because the sessions's Close method is often not called, and
	// without a finished parent span child spans seem to often be left orphaned.
	ll.sessionSpan.Finish()
}
func (ll *lampshadeLifecycleListener) OnClientInitWritten(context.Context)          {}
func (ll *lampshadeLifecycleListener) OnClientInitRead(context.Context)             {}
func (ll *lampshadeLifecycleListener) OnSessionError(readErr error, writeErr error) {}

func (ll *lampshadeLifecycleListener) OnStreamInit(ctx context.Context, id uint16) lampshade.StreamLifecycleListener {
	// This is somewhat confusing as a result of the call to DialContext creating a new session
	// as necessary. As a result, this lifecycle listener sometimes has a
	opts := make([]opentracing.StartSpanOption, 0)
	var span opentracing.Span
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
		span = opentracing.GlobalTracer().StartSpan(fmt.Sprintf("stream-%v-%v", id, ll.upstreamHost), opts...)
	} else {
		noop := opentracing.NoopTracer{}
		span = noop.StartSpan("noop")
	}
	return newLampshadeStreamListener(span)
}
func (ls *lampshadeStreamListener) OnStreamRead(num int) {
	ls.span.LogFields(otlog.Int("r", num))
}
func (ls *lampshadeStreamListener) OnStreamWrite(num int) {
	ls.span.LogFields(otlog.Int("w", num))
}
func (ls *lampshadeStreamListener) OnStreamClose() {
	ls.span.Finish()
}

type lampshadeStreamListener struct {
	span opentracing.Span
}
