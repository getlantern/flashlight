// +build !iosapp

package chained

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	goerrors "errors"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"strconv"
	"sync"

	"github.com/getlantern/errors"
	shadowsocks "github.com/getlantern/lantern-shadowsocks/client"

	"github.com/getlantern/flashlight/ops"
)

const (
	defaultShadowsocksUpstreamSuffix = "test"
)

type shadowsocksImpl struct {
	reportDialCore reportDialCoreFn
	client         shadowsocks.Client
	upstream       string
	rng            *mrand.Rand
	rngmx          sync.Mutex
}

func newShadowsocksImpl(name, addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	secret := s.ptSetting("shadowsocks_secret")
	cipher := s.ptSetting("shadowsocks_cipher")
	upstream := s.ptSetting("shadowsocks_upstream")
	if upstream == "" {
		upstream = defaultShadowsocksUpstreamSuffix
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, errors.New("unable to parse port in address %v: %v", addr, err)
	}
	client, err := shadowsocks.NewClient(host, port, secret, cipher)
	if err != nil {
		return nil, errors.New("failed to create shadowsocks client: %v", err)
	}

	var seed int64
	err = binary.Read(crand.Reader, binary.BigEndian, &seed)
	if err != nil {
		return nil, errors.New("unable to initialize rng: %v", err)
	}
	source := mrand.NewSource(seed)
	rng := mrand.New(source)

	return &shadowsocksImpl{
		reportDialCore: reportDialCore,
		client:         client,
		upstream:       upstream,
		rng:            rng,
	}, nil
}

func (impl *shadowsocksImpl) close() {
}

func (impl *shadowsocksImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		conn, err := impl.client.DialTCP(nil, impl.generateUpstream())
		if err != nil {
			return nil, err
		}
		return &ssWrapConn{conn}, nil
	})
}

// generateUpstream() creates a marker upstream address.  This isn't an
// acutal upstream that will be dialed, it signals that the upstream
// should be determined by other methods.  It's just a bit random just to
// mix it up and not do anything especially consistent on every dial.
//
// To satisy shadowsocks expectations, a small random string is prefixed onto the
// configured suffix (along with a .) and a port is affixed to the end.
func (impl *shadowsocksImpl) generateUpstream() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	impl.rngmx.Lock()
	defer impl.rngmx.Unlock()
	// [2 - 22]
	sz := 2 + impl.rng.Intn(21)
	b := make([]byte, sz)
	for i := range b {
		b[i] = letters[impl.rng.Intn(len(letters))]
	}
	return fmt.Sprintf("%s.%s:443", string(b), impl.upstream)
}

// this is a helper to smooth out error bumps
// that the rest of lantern doesn't really expect, but happen
// in the shadowsocks impl when closing.
type ssWrapConn struct {
	net.Conn
}

func (c *ssWrapConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	return n, ssTranslateError(err)
}

func (c *ssWrapConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	return n, ssTranslateError(err)
}

func ssTranslateError(err error) error {
	if err == nil {
		return nil
	}

	if goerrors.Is(err, net.ErrClosed) {
		return io.EOF
	}

	return err
}
