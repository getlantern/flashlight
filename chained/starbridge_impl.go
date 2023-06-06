package chained

import (
	"context"
	"fmt"
	"net"

	replicant "github.com/OperatorFoundation/Replicant-go/Replicant/v3"
	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/polish"
	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/toneburst"
	"github.com/OperatorFoundation/Starbridge-go/Starbridge/v3"

	"github.com/getlantern/flashlight/v7/ops"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	"github.com/getlantern/netx"
)

// The starbridge client and server authenticate (amongst other things) the server address during
// the handshake. The problem with this is that our proxies do not necessarily listen on their
// public IP address (particularly in the triangle routing case). To get around this, we use the
// same fake server address on both client and server. To be clear, nothing binds on or dials this
// address. It is used exclusively for coordinating client and server authentication.
//
// We are not concerned about degradation of security here. The keypair exchanged out-of-band should
// be sufficient to authenticate the proxy. The address authentication scheme provides no security
// anyway; a bad actor could set the authentication address to any value they want, just as we do
// here.
//
// As an entry point to this logic, see:
// https://github.com/OperatorFoundation/go-shadowsocks2/blob/v1.1.12/darkstar/client.go#L189
const fakeListenAddr = "1.2.3.4:5678"

type starbridge struct {
	nopCloser
	addr           string
	config         replicant.ClientConfig
	reportDialCore reportDialCoreFn
}

func (impl *starbridge) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	tcpConn, err := impl.reportDialCore(op, func() (net.Conn, error) {
		return netx.DialContext(ctx, "tcp", impl.addr)
	})
	if err != nil {
		return nil, err
	}

	conn, err := Starbridge.NewClientConnection(impl.config, tcpConn)
	if err != nil {
		return nil, fmt.Errorf("starbridge wrapping err: %w", err)
	}
	return conn, nil
}

func newStarbridgeImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	if ptSetting(pc, "starbridge_public_key") == "" {
		return nil, errors.New("no public key")
	}

	cfg := getClientConfig(ptSetting(pc, "starbridge_public_key"))

	return &starbridge{
		addr:           addr,
		config:         cfg,
		reportDialCore: reportDialCore,
	}, nil
}

// Adapted from https://github.com/OperatorFoundation/Starbridge-go/blob/v3.0.12/Starbridge/v3/starbridge.go#L237-L253
func getClientConfig(serverPublicKey string) replicant.ClientConfig {
	polishClientConfig := polish.DarkStarPolishClientConfig{
		ServerAddress:   fakeListenAddr,
		ServerPublicKey: serverPublicKey,
	}

	toneburstClientConfig := toneburst.StarburstConfig{
		Mode: "SMTPClient",
	}

	clientConfig := replicant.ClientConfig{
		Toneburst: toneburstClientConfig,
		Polish:    polishClientConfig,
	}

	return clientConfig
}
