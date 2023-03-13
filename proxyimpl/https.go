package proxyimpl

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/browsers/simbrowser"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/hellosplitter"
	"github.com/getlantern/tlsdialer/v3"
	"github.com/getlantern/tlsresumption"
)

type httpsImpl struct {
	common.NopCloser
	dialCore                coreDialer
	addr                    string
	tlsConfig               *tls.Config
	roller                  *helloRoller
	tlsClientHelloSplitting bool
	sync.Mutex
}

func newHTTPSImpl(configDir, name, addr string, pc *config.ProxyConfig, uc common.UserConfig, dialCore coreDialer) (ProxyImpl, error) {
	const timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tlsConfig, hellos, err := tlsConfigForProxy(ctx, configDir, name, pc, uc)
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	if len(hellos) == 0 {
		return nil, log.Error(errors.New("expected at least one hello"))
	}
	return &httpsImpl{
		dialCore:                dialCore,
		addr:                    addr,
		tlsConfig:               tlsConfig,
		roller:                  &helloRoller{hellos: hellos},
		tlsClientHelloSplitting: pc.TLSClientHelloSplitting,
	}, nil
}

func (impl *httpsImpl) DialServer(
	op *ops.Op,
	ctx context.Context,
	dialer Dialer) (net.Conn, error) {
	r := impl.roller.getCopy()
	defer impl.roller.updateTo(r)

	currentHello := r.current()
	helloID, helloSpec, err := currentHello.utlsSpec()
	if err != nil {
		log.Debugf("failed to generate valid utls hello spec; advancing roller: %v", err)
		r.advance()
		return nil, errors.New("failed to generate valid utls hello spec: %v", err)
	}
	d := tlsdialer.Dialer{
		DoDial: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			tcpConn, err := impl.dialCore(op, ctx, impl.addr)
			if err != nil {
				return nil, err
			}
			if impl.tlsClientHelloSplitting {
				tcpConn = hellosplitter.Wrap(tcpConn, splitClientHello)
			}

			// Run the post-layer 4 dial function if specified
			// if onPostLayer4Dial != nil {
			// 	tcpConn, err = onPostLayer4Dial(tcpConn.(*net.TCPConn))
			// 	if err != nil {
			// 		return nil, fmt.Errorf("onPostLayer4Dial failed: %v", err)
			// 	}
			// }
			return tcpConn, err
		},
		Timeout:         timeoutFor(ctx),
		SendServerName:  impl.tlsConfig.ServerName != "",
		Config:          impl.tlsConfig.Clone(),
		ClientHelloID:   helloID,
		ClientHelloSpec: helloSpec,
	}
	result, err := d.DialForTimings("tcp", impl.addr)
	if err != nil {
		if isHelloErr(err) {
			log.Debugf("got error likely related to bad hello; advancing roller: %v", err)
			r.advance()
		}
		return nil, err
	}
	return result.Conn, nil
}

func timeoutFor(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		return deadline.Sub(time.Now())
	}
	return ChainedDialTimeout
}

// Generates TLS configuration for connecting to proxy specified by the config.ProxyConfig. This
// function may block while determining things like how to mimic the default browser's client hello.
//
// Returns a slice of ClientHellos to be used for dialing. These hellos are in priority order: the
// first hello is the "ideal" one and the remaining hellos serve as backup in case something is
// wrong with the previous hellos. There will always be at least one hello. For each hello, the
// ClientHelloSpec will be non-nil if and only if the ClientHelloID is tls.HelloCustom.
func tlsConfigForProxy(ctx context.Context, configDir, proxyName string, pc *config.ProxyConfig, uc common.UserConfig) (
	*tls.Config, []helloSpec, error) {

	configuredHelloID := clientHelloID(pc)
	var ss *tls.ClientSessionState
	var err error
	if pc.TLSClientSessionState != "" {
		ss, err = tlsresumption.ParseClientSessionState(pc.TLSClientSessionState)
		if err != nil {
			log.Errorf("Unable to parse serialized client session state, continuing with normal handshake: %v", err)
		} else {
			log.Debug("Using serialized client session state")
			if configuredHelloID.Client == "Golang" {
				log.Debug("Need to mimic browser hello for session resumption, defaulting to HelloChrome_Auto")
				configuredHelloID = tls.HelloChrome_Auto
			}
		}
	}

	configuredHelloSpec := helloSpec{configuredHelloID, nil}
	if configuredHelloID == helloBrowser {
		configuredHelloSpec = getBrowserHello(ctx, configDir, uc)
	}

	sessionTTL := simbrowser.ChooseForUser(ctx, uc).SessionTicketLifetime
	sessionCache := newExpiringSessionCache(proxyName, sessionTTL, ss)

	// We configure cipher suites here, then specify later which ClientHello type to mimic. As the
	// ClientHello type itself specifies cipher suites, it's not immediately obvious who wins.
	//
	// When we specify HelloGolang (a fallback at the time of this writing), the cipher suites in
	// the config are used. For our purposes, this is preferable, particularly as we can configure
	// these remotely.
	//
	// For all other hello types, the cipher suite in the ClientHelloSpec are used:
	//  1. With HelloCustom, we call ApplyPreset manually; skip to step 4.
	//  2. The handshake is built by BuildHandshakeState, which calls applyPresetByID:
	//     https://github.com/getlantern/utls/blob/1abdc4b1acab98e8776ae9a5201f67968ffa01dc/u_conn.go#L82
	//  3. For HelloRandomized*, random suites are generated by generateRandomSpec. For other,
	//     non-custom hellos, ApplyPreset is called:
	//     https://github.com/getlantern/utls/blob/1abdc4b1acab98e8776ae9a5201f67968ffa01dc/u_parrots.go#L977
	//  4. ApplyPreset configures the cipher suites according to the ClientHelloSpec:
	//     https://github.com/getlantern/utls/blob/1abdc4b1acab98e8776ae9a5201f67968ffa01dc/u_parrots.go#L1034-L1040

	cipherSuites := orderedCipherSuitesFromConfig(pc)

	// Proxy certs are self-signed. We will just verify that the peer (the proxy) provided exactly
	// the expected certificate.
	if pc.Cert == "" {
		return nil, nil, errors.New("no proxy certificate configured")
	}
	block, rest := pem.Decode([]byte(pc.Cert))
	if block == nil {
		return nil, nil, errors.New("failed to decode proxy certificate as PEM block")
	}
	if len(rest) > 0 {
		return nil, nil, errors.New("unexpected extra data in proxy certificate PEM")
	}
	if block.Type != "CERTIFICATE" {
		return nil, nil, errors.New("expected certificate in PEM block")
	}
	proxyCertDER := block.Bytes

	// Byte-wise comparsion, verifying that the proxy cert is the expected one.
	// n.b. Not invoked when resuming a session (as there are no peer certificates to inspect).
	verifyPeerCert := func(peerCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(peerCerts) == 0 {
			return errors.New("no peer certificate")
		}
		if !bytes.Equal(peerCerts[0], proxyCertDER) {
			return errors.New("peer certificate does not match expected")
		}
		return nil
	}

	cfg := &tls.Config{
		ClientSessionCache: sessionCache,
		CipherSuites:       cipherSuites,
		ServerName:         pc.TLSServerNameIndicator,
		KeyLogWriter:       getTLSKeyLogWriter(),

		// We have to disable standard verification because we want to provide an alternative SNI.
		// We provide our own verification function, which ensures that verification still occurs
		// as part of the handshake.
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: verifyPeerCert,
	}
	hellos := []helloSpec{
		configuredHelloSpec,
		{tls.HelloChrome_Auto, nil},
		{tls.HelloGolang, nil},
	}

	return cfg, hellos, nil
}
