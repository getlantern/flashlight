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
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
	"github.com/getlantern/lampshade"
	"github.com/getlantern/netx"
)

type lampshadeImpl struct {
	nopCloser
	name   string
	addr   string
	dialer lampshade.Dialer
	setOp  func(op *ops.Op)
}

func newLampshadeImpl(name, addr string, s *ChainedServerInfo) (proxyImpl, error) {
	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(s.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
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
	setOp := func(op *ops.Op) {
		op.Set("ls_win", windowSize).
			Set("ls_pad", maxPadding).
			Set("ls_streams", int(maxStreamsPerConn)).
			Set("ls_cipher", cipherCode.String())
	}
	return &lampshadeImpl{name: name, addr: addr, dialer: dialer, setOp: setOp}, nil
}

func (impl *lampshadeImpl) dialServer(op *ops.Op, ctx context.Context, dialCore dialCoreFn) (net.Conn, error) {
	impl.setOp(op)
	return impl.dialer.DialContext(ctx, func() (net.Conn, error) {
		// note - we do not wrap the TCP connection with IdleTiming because
		// lampshade cleans up after itself and won't leave excess unused
		// connections hanging around.
		log.Debugf("Dialing lampshade TCP connection to %v", impl.name)
		return dialCore(op, ctx)
	})
}

func (impl *lampshadeImpl) dialCore(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return netx.DialTimeout("tcp", impl.addr, timeoutFor(ctx))
}
