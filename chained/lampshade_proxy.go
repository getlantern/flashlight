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
)

const lampshadeTransport = "lampshade"

type lampshadeProxy struct {
	log   golog.Logger
	s     *ChainedServerInfo
	name  string
	proto string
}

func newLampshadeProxy(s *ChainedServerInfo, name, proto string) *lampshadeProxy {
	return &lampshadeProxy{
		name:  name,
		proto: proto,
		s:     s,
		log:   golog.LoggerFor("chained.lampshade"),
	}
}

func (l *lampshadeProxy) newProxy(name, proto string, s *ChainedServerInfo, uc common.UserConfig) (*proxy, error) {
	return newProxy(l.name, lampshadeTransport, l.proto, l.s, uc, s.Trusted, false, l)
}

func (l *lampshadeProxy) dialServer(ctx context.Context, p *proxy) (net.Conn, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(l.s.Cert))
	if err != nil {
		return nil, l.log.Error(errors.Wrap(err).With("addr", l.s.Addr))
	}
	rsaPublicKey, ok := cert.X509().PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not an RSA public key")
	}
	cipherCode := lampshade.Cipher(l.s.ptSettingInt(fmt.Sprintf("cipher_%v", runtime.GOARCH)))
	if cipherCode == 0 {
		if runtime.GOARCH == "amd64" {
			// On 64-bit Intel, default to AES128_GCM which is hardware accelerated
			cipherCode = lampshade.AES128GCM
		} else {
			// default to ChaCha20Poly1305 which is fast even without hardware acceleration
			cipherCode = lampshade.ChaCha20Poly1305
		}
	}
	windowSize := l.s.ptSettingInt("windowsize")
	maxPadding := l.s.ptSettingInt("maxpadding")
	maxStreamsPerConn := uint16(l.s.ptSettingInt("streams"))
	idleInterval, parseErr := time.ParseDuration(l.s.ptSetting("idleinterval"))
	if parseErr != nil || idleInterval < 0 {
		// This should be less than the server's IdleTimeout to avoid trying to use
		// a connection that was just idled. The client's IdleTimeout is already set
		// appropriately for this purpose, so use that.
		idleInterval = IdleTimeout
		l.log.Debugf("%s: defaulted idleinterval to %v", l.name, idleInterval)
	}
	pingInterval, parseErr := time.ParseDuration(l.s.ptSetting("pinginterval"))
	if parseErr != nil || pingInterval < 0 {
		pingInterval = 15 * time.Second
		l.log.Debugf("%s: defaulted pinginterval to %v", l.name, pingInterval)
	}
	maxLiveConns := l.s.ptSettingInt("maxliveconns")
	if maxLiveConns <= 0 {
		maxLiveConns = 5
		l.log.Debugf("%s: defaulted maxliveconns to %v", l.name, maxLiveConns)
	}
	redialSessionInterval, parseErr := time.ParseDuration(l.s.ptSetting("redialsessioninterval"))
	if parseErr != nil || redialSessionInterval < 0 {
		redialSessionInterval = 5 * time.Second
		l.log.Debugf("%s: defaulted redialsessioninterval to %v", l.name, redialSessionInterval)
	}
	dialer := lampshade.NewDialer(&lampshade.DialerOpts{
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
	})
	return p.reportedDial(l.s.Addr, lampshadeTransport, l.proto, func(op *ops.Op) (net.Conn, error) {
		op.Set("ls_win", windowSize).
			Set("ls_pad", maxPadding).
			Set("ls_streams", int(maxStreamsPerConn)).
			Set("ls_cipher", cipherCode.String())
		conn, err := dialer.DialContext(ctx, func() (net.Conn, error) {
			// note - we do not wrap the TCP connection with IdleTiming because
			// lampshade cleans up after itself and won't leave excess unused
			// connections hanging around.
			l.log.Debugf("Dialing lampshade TCP connection to %v", p.Label())
			return p.dialCore(op)(ctx)
		})
		return overheadWrapper(true)(conn, err)
	})
}

func (l *lampshadeProxy) dialOrigin(op *ops.Op, ctx context.Context, p *proxy, network, addr string) (net.Conn, error) {
	return defaultDialOrigin(op, ctx, p, network, addr)
}
