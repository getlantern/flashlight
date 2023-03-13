package proxyimpl

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

	"github.com/Jigsaw-Code/outline-ss-server/client"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"

	"github.com/getlantern/flashlight/chained/prefixgen"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
)

const (
	defaultShadowsocksUpstreamSuffix = "test"
)

type shadowsocksImpl struct {
	common.NopCloser
	reportDialCore ReportDialCoreFn
	client         client.Client
	upstream       string
	rng            *mrand.Rand
	rngmx          sync.Mutex
}

func newShadowsocksImpl(name, addr string, pc *config.ProxyConfig, reportDialCore ReportDialCoreFn) (ProxyImpl, error) {
	secret := ptSetting(pc, "shadowsocks_secret")
	cipher := ptSetting(pc, "shadowsocks_cipher")
	upstream := ptSetting(pc, "shadowsocks_upstream")
	prefixGen := ptSetting(pc, "shadowsocks_prefix_generator")

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
	cl, err := client.NewClient(host, port, secret, cipher)
	if err != nil {
		return nil, errors.New("failed to create shadowsocks client: %v", err)
	}

	// Infrastructure python code seems to insert "None" as the prefix generator if there is none.
	if prefixGen != "" && prefixGen != "None" {
		gen, err := prefixgen.New(prefixGen)
		if err != nil {
			log.Errorf("failed to parse shadowsocks prefix generator from %v for proxy %v: %v", prefixGen, name, err)
		} else {
			cl.SetTCPSaltGenerator(client.NewPrefixSaltGenerator(gen()))
		}
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
		client:         cl,
		upstream:       upstream,
		rng:            rng,
	}, nil
}

func (impl *shadowsocksImpl) DialServer(
	op *ops.Op,
	ctx context.Context,
	dialer Dialer) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		conn, err := impl.client.DialTCP(nil, impl.generateUpstream(), dialer)
		// func(network string, laddr, raddr *net.TCPAddr) (net.Conn, error) {
		// 	c, err := net.DialTCP("tcp", laddr, raddr)
		// 	if err != nil {
		// 		return nil, fmt.Errorf("Unable to dial proxy: %v", err)
		// 	}
		// 	// TODO <04-04-2023, soltzen> This just means wrap the
		// 	// connection with the prefix. Ideally, don't wrap at all and
		// 	// just make a custom dialer.
		// 	wrappedConn, err := onPostLayer4Dial(c)
		// 	if err != nil {
		// 		return nil, fmt.Errorf("Unable to wrap connection with PostLayer4Dial: %v", err)
		// 	}
		// 	return wrappedConn, nil
		// },
		// )
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
