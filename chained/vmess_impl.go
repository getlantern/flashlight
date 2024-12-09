package chained

import (
	"context"
	mrand "math/rand/v2"
	"net"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"
	vmess "github.com/getlantern/sing-vmess"
	N "github.com/getlantern/sing-vmess/network"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/metadata"

	"github.com/getlantern/flashlight/v7/ops"
)

type vmessImpl struct {
	reportDialCore reportDialCoreFn
	client         *vmess.Client
	uuid           string
	addr           string
}

// newVmessImpl creates a new VMess proxy implementation.
// supports the following options:
//   - uuid: the UUID of the user on the VMess server
//   - security: the security level to use, defaults to "auto".  Options are:
//     "auto": automatically determine the security level
//     "none" or "zero": no security
//     "aes-128-cfb": legacy security
//     "aes-128-gcm": AES-128-GCM security
//     "chacha20-poly1305": ChaCha20-Poly1305 security
func newVmessImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	uuid := ptSetting(pc, "uuid")
	security := ptSetting(pc, "security")
	if security == "" {
		security = "auto"
	}

	client, err := vmess.NewClient(uuid, security, 0)

	if err != nil {
		return nil, errors.New("failed to create vmess client: %v", err)
	}

	return &vmessImpl{
		reportDialCore: reportDialCore,
		client:         client,
		uuid:           uuid,
		addr:           addr,
	}, nil
}

func (impl *vmessImpl) close() {
}

func (impl *vmessImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		target := metadata.ParseSocksaddrHostPort(impl.generateUpstream(), 443)
		conn, err := (&net.Dialer{}).DialContext(ctx, N.NetworkTCP, impl.addr)
		if err != nil {
			common.Close(conn)
			return nil, err
		}
		return impl.client.DialEarlyConn(conn, target), nil
	})
}

// generateUpstream() creates a marker upstream address.  This isn't an
// acutal upstream that will be dialed, it signals that the upstream
// should be determined by other methods.  It's just a bit random just to
// mix it up and not do anything especially consistent on every dial.
func (impl *vmessImpl) generateUpstream() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// [2 - 22]
	sz := 2 + mrand.IntN(21)
	b := make([]byte, sz)
	for i := range b {
		b[i] = letters[mrand.IntN(len(letters))]
	}
	return string(b) + ".com"
}
