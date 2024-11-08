package chained

import (
	"context"
	crand "crypto/rand"
	"crypto/x509"
	"encoding/binary"
	goerrors "errors"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"strconv"
	"sync"

	tls "github.com/refraction-networking/utls"

	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/transport/shadowsocks"

	"github.com/getlantern/common/config"
	"github.com/getlantern/errors"

	"github.com/getlantern/flashlight/v7/chained/prefixgen"
	"github.com/getlantern/flashlight/v7/ops"
)

const (
	defaultShadowsocksUpstreamSuffix = "test"
)

type shadowsocksImpl struct {
	reportDialCore reportDialCoreFn
	client         *shadowsocks.StreamDialer
	upstream       string
	rng            *mrand.Rand
	rngmx          sync.Mutex
	tlsConfig      *tls.Config
	nopCloser
}

type PrefixSaltGen struct {
	prefixFunc func() ([]byte, error)
}

func (p *PrefixSaltGen) GetSalt(salt []byte) error {
	prefix, err := p.prefixFunc()
	if err != nil {
		return fmt.Errorf("failed to generate prefix: %v", err)
	}
	n := copy(salt, prefix)
	if n != len(prefix) {
		return errors.New("prefix is too long")
	}
	_, err = crand.Read(salt[n:])
	return err
}

func newShadowsocksImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	secret := ptSetting(pc, "shadowsocks_secret")
	cipher := ptSetting(pc, "shadowsocks_cipher")
	upstream := ptSetting(pc, "shadowsocks_upstream")
	prefixGen := ptSetting(pc, "shadowsocks_prefix_generator")
	withTLSStr := ptSetting(pc, "shadowsocks_with_tls")
	withTLS, err := strconv.ParseBool(withTLSStr)
	if err != nil {
		withTLS = false
	}

	if upstream == "" {
		upstream = defaultShadowsocksUpstreamSuffix
	}

	key, err := shadowsocks.NewEncryptionKey(cipher, secret)
	if err != nil {
		return nil, errors.New("failed to create shadowsocks key: %v", err)
	}

	cl, err := shadowsocks.NewStreamDialer(&transport.TCPEndpoint{Address: addr}, key)
	if err != nil {
		return nil, errors.New("failed to create shadowsocks client: %v", err)
	}

	// Infrastructure python code seems to insert "None" as the prefix generator if there is none.
	if prefixGen != "" && prefixGen != "None" {
		gen, err := prefixgen.New(prefixGen)
		if err != nil {
			log.Errorf("failed to parse shadowsocks prefix generator from %v for proxy %v: %v", prefixGen, name, err)
			return nil, errors.New("failed to parse shadowsocks prefix generator from %v for proxy %v: %v", prefixGen, name, err)
		}
		prefixFunc := func() ([]byte, error) { return gen(), nil }
		cl.SaltGenerator = &PrefixSaltGen{prefixFunc}
	}

	var seed int64
	err = binary.Read(crand.Reader, binary.BigEndian, &seed)
	if err != nil {
		return nil, errors.New("unable to initialize rng: %v", err)
	}
	source := mrand.NewSource(seed)
	rng := mrand.New(source)

	var tlsConfig *tls.Config = nil
	if withTLS {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(pc.Cert)); !ok {
			return nil, errors.New("couldn't add certificate to pool")
		}
		ip, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.New("couldn't split host and port: %v", err)
		}

		tlsConfig = &tls.Config{
			RootCAs:    certPool,
			ServerName: ip,
		}
	}

	return &shadowsocksImpl{
		reportDialCore: reportDialCore,
		client:         cl,
		upstream:       upstream,
		rng:            rng,
		tlsConfig:      tlsConfig,
	}, nil
}

func (impl *shadowsocksImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		conn, err := impl.client.DialStream(ctx, impl.generateUpstream())
		if err != nil {
			return nil, err
		}
		if impl.tlsConfig != nil {
			tlsConn := tls.Client(conn, impl.tlsConfig)
			return &ssWrapConn{tlsConn}, nil
		}
		return &ssWrapConn{conn}, nil
	})
}

func (*shadowsocksImpl) isReady() (bool, error) {
	return true, nil
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
