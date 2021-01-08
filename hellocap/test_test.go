package hellocap

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlantern/tlsmasq"
	"github.com/getlantern/tlsmasq/ptlshs"
	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

const (
// sni    = "stream.ru"
// origin = "stream.ru:443"

// sni    = "google.com"
// origin = "google.com:443"
)

var (
	// suites = []uint16{
	// 	0xc030, 0xcca8, 0xc02f, 0xc028, 0xc027, 0xc014, 0xc013,
	// 	0x009d, 0x009c, 0x003d, 0x003c, 0x0035, 0x002f,
	// }
	// suites = []uint16{0xc02f}
	secret = ptlshs.Secret{}
)

// TODO:
//  - simulate tlsmasq proxy which was failing yesterday
//	- try to hit with Go tls
//	- try to hit with utls
//		- with HelloChrome_83
//		- with genspec output
//		- with selection from tlsConfigForProxy
// 	- try playing with TLS 1.2 vs 1.3

type origin struct {
	hostname, sni string
	suites        []uint16
}

func startProxy(t *testing.T, addr string) (proxyAddr string) {
	t.Helper()

	nonFatalErrors := make(chan error)
	go func() {
		for err := range nonFatalErrors {
			t.Log("non-fatal error:", err)
		}
	}()
	t.Cleanup(func() { close(nonFatalErrors) })

	cfg := tlsmasq.ListenerConfig{
		ProxiedHandshakeConfig: ptlshs.ListenerConfig{
			DialOrigin:     func() (net.Conn, error) { return net.Dial("tcp", addr) },
			Secret:         secret,
			NonFatalErrors: nonFatalErrors,
		},
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert, rsaCert},
		},
	}
	l, err := tlsmasq.Listen("tcp", "", cfg)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })

	testComplete := make(chan struct{})
	t.Cleanup(func() { close(testComplete) })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil && strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			if err != nil {
				t.Log("accept error:", err)
				continue
			}
			go func(c net.Conn) {
				if err := c.(tlsmasq.Conn).Handshake(); err != nil {
					select {
					case <-testComplete:
					default:
						t.Log("server handshake error:", err)
					}
				}
			}(conn)
		}
	}()

	return l.Addr().String()
}

func testWithShaker(o origin, hs ptlshs.Handshaker) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()
		// t.Parallel()

		proxyAddr := startProxy(t, o.hostname+":443")
		cfg := tlsmasq.DialerConfig{
			ProxiedHandshakeConfig: ptlshs.DialerConfig{
				Secret:     secret,
				Handshaker: hs,
			},
			TLSConfig: &tls.Config{
				ServerName:         o.sni,
				CipherSuites:       o.suites,
				InsecureSkipVerify: true,
			},
		}
		conn, err := tlsmasq.DialTimeout("tcp", proxyAddr, cfg, 2*time.Second)
		require.NoError(t, err)
		defer conn.Close()

		require.NoError(t, conn.(tlsmasq.Conn).Handshake())
	}
}

func TestTest(t *testing.T) {
	// o := origin{"stream.ru", "stream.ru", []uint16{
	// 	0xc030, 0xcca8, 0xc02f, 0xc028, 0xc027, 0xc014, 0xc013,
	// 	0x009d, 0x009c, 0x003d, 0x003c, 0x0035, 0x002f,
	// }}
	o := origin{"planetlabor.com", "www.planetlabor.com", []uint16{
		0xc030, 0x009f, 0xc02f, 0x009e, 0xc028, 0x006b, 0xc027, 0x0067, 0xc014,
		0x0039, 0xc013, 0x0033, 0x009d, 0x009c, 0x003d, 0x003c, 0x0035, 0x002f,
	}}

	// t.Run("stdlib", testWithShaker(ptlshs.StdLibHandshaker{
	// 	Config: &tls.Config{
	// 		ServerName:   sni,
	// 		CipherSuites: suites,
	// 		MaxVersion:   tls.VersionTLS12,
	// 	},
	// }))
	// t.Run("hello golang", testWithShaker(&utlsHandshaker{
	// 	cfg: &utls.Config{
	// 		ServerName:   sni,
	// 		CipherSuites: suites,
	// 		MaxVersion:   tls.VersionTLS12,
	// 	},
	// 	roller: &helloRoller{
	// 		hellos: []hello{{utls.HelloGolang, nil}},
	// 	},
	// }))
	t.Run("chrome83", testWithShaker(o, &utlsHandshaker{
		cfg: &utls.Config{
			ServerName:   o.sni,
			CipherSuites: o.suites,
			MaxVersion:   tls.VersionTLS12,
		},
		roller: &helloRoller{
			hellos: []hello{{utls.HelloChrome_83, nil}},
		},
	}))
}

func TestAllOrigins(t *testing.T) {
	const originsFile = "/Users/harryharpham/Desktop/tmp/origins.txt"

	decodeUint16 := func(s string) (uint16, error) {
		b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint16(b), nil
	}

	parseOriginLine := func(line string) (*origin, error) {
		splits := strings.Split(line, ";")
		if len(splits) != 3 {
			return nil, fmt.Errorf("expected 3 segments, got %d", len(splits))
		}

		suites := []uint16{}
		suiteStrings := strings.Split(splits[2], ",")
		if len(suiteStrings) == 1 && suiteStrings[0] == "" {
			return nil, errors.New("no cipher suites specified")
		}
		for _, s := range suiteStrings {
			suite, err := decodeUint16(s)
			if err != nil {
				return nil, fmt.Errorf("bad cipher string '%s': %v", s, err)
			}
			suites = append(suites, suite)
		}
		return &origin{splits[0], splits[1], suites}, nil
	}

	f, err := os.Open(originsFile)
	require.NoError(t, err)
	defer f.Close()

	origins := []origin{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		o, err := parseOriginLine(scanner.Text())
		require.NoError(t, err)
		origins = append(origins, *o)
	}

	for _, o := range origins {
		t.Run(o.hostname, testWithShaker(o, &utlsHandshaker{
			cfg: &utls.Config{
				ServerName:   o.sni,
				CipherSuites: o.suites,
				MaxVersion:   tls.VersionTLS12,
			},
			roller: &helloRoller{
				hellos: []hello{{utls.HelloChrome_83, nil}},
			},
		}))
	}
}

func TestTest2(t *testing.T) {
	l, err := tls.Listen("tcp", "", &tls.Config{
		Certificates: []tls.Certificate{rsaCert, cert},
		CipherSuites: []uint16{0xc02b},
		MaxVersion:   tls.VersionTLS12,
	})
	require.NoError(t, err)
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		if err := conn.(*tls.Conn).Handshake(); err != nil {
			panic(err)
		}
	}()

	conn, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{
		// ServerName:         sni,
		CipherSuites:       []uint16{0xc02b},
		MaxVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.Handshake())
	fmt.Printf("%#x\n", conn.ConnectionState().Version)
	fmt.Printf("%#x\n", conn.ConnectionState().CipherSuite)
}

func TestNormalHandshake(t *testing.T) {
	conn, err := tls.Dial("tcp", "mdusd.org:443", &tls.Config{
		ServerName: "mdusd.org",
		CipherSuites: []uint16{
			0xc02c, 0xc030, 0xcca9, 0xcca8, 0xc02b, 0xc02f, 0xc024, 0xc028, 0xc023,
			0xc027, 0xc014, 0xc013, 0x009d, 0x009c, 0x003d, 0x003c, 0x0035, 0x002f,
		},
		MaxVersion: tls.VersionTLS12,
	})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.Handshake())
	fmt.Printf("cipher suite: %#x\n", conn.ConnectionState().CipherSuite)
	fmt.Printf("version: %#x\n", conn.ConnectionState().Version)
}

func TestUTLSHandshake(t *testing.T) {
	handshaker := &utlsHandshaker{
		cfg: &utls.Config{
			ServerName: "mdusd.org",
			CipherSuites: []uint16{
				0xc02c, 0xc030, 0xcca9, 0xcca8, 0xc02b, 0xc02f, 0xc024, 0xc028, 0xc023,
				0xc027, 0xc014, 0xc013, 0x009d, 0x009c, 0x003d, 0x003c, 0x0035, 0x002f,
			},
			MaxVersion: tls.VersionTLS12,
		},
		roller: &helloRoller{
			hellos: []hello{{utls.HelloChrome_Auto, nil}},
		},
	}

	conn, err := net.Dial("tcp", "mdusd.org:443")
	require.NoError(t, err)
	defer conn.Close()

	res, err := handshaker.Handshake(conn)
	require.NoError(t, err)
	fmt.Printf("negotiated version: %#x\n", res.Version)
	fmt.Printf("negotiated suite: %#x\n", res.CipherSuite)
}

// utlsHandshaker implements tlsmasq/ptlshs.Handshaker. This allows us to parrot browsers like
// Chrome in our handshakes with tlsmasq origins.
type utlsHandshaker struct {
	cfg    *utls.Config
	roller *helloRoller
	sync.Mutex
}

func (h *utlsHandshaker) Handshake(conn net.Conn) (*ptlshs.HandshakeResult, error) {
	r := h.roller.getCopy()
	defer h.roller.updateTo(r)

	isHelloErr := func(err error) bool {
		if strings.Contains(err.Error(), "hello spec") {
			// These errors are created below.
			return true
		}
		if strings.Contains(err.Error(), "tls: ") {
			// A TLS-level error is likely related to a bad hello.
			return true
		}
		return false
	}

	currentHello := r.current()
	uconn := utls.UClient(conn, h.cfg, currentHello.id)
	res, err := func() (*ptlshs.HandshakeResult, error) {
		if currentHello.id == utls.HelloCustom {
			if currentHello.spec == nil {
				return nil, errors.New("hello spec must be provided if HelloCustom is used")
			}
			if err := uconn.ApplyPreset(currentHello.spec); err != nil {
				return nil, fmt.Errorf("failed to set custom hello spec: %w", err)
			}
		}
		if err := uconn.Handshake(); err != nil {
			return nil, err
		}
		return &ptlshs.HandshakeResult{
			Version:     uconn.ConnectionState().Version,
			CipherSuite: uconn.ConnectionState().CipherSuite,
		}, nil
	}()
	if err != nil && isHelloErr(err) {
		fmt.Println("got error likely related to bad hello; advancing roller:", err)
		r.advance()
	}
	return res, err
}

var (
	rsaCertPEM = []byte(`-----BEGIN CERTIFICATE-----
MIICnjCCAgegAwIBAgIURmEo2XMMETzOCFWqk5STHiWQU+swDQYJKoZIhvcNAQEL
BQAwYTELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAkNPMRUwEwYDVQQHDAxGb3J0IENv
bGxpbnMxDDAKBgNVBAoMA0JOUzEMMAoGA1UECwwDUiZEMRIwEAYDVQQDDAlsb2Nh
bGhvc3QwHhcNMjAxMTA1MTQzNTEwWhcNMjAxMjA1MTQzNTEwWjBhMQswCQYDVQQG
EwJVUzELMAkGA1UECAwCQ08xFTATBgNVBAcMDEZvcnQgQ29sbGluczEMMAoGA1UE
CgwDQk5TMQwwCgYDVQQLDANSJkQxEjAQBgNVBAMMCWxvY2FsaG9zdDCBnzANBgkq
hkiG9w0BAQEFAAOBjQAwgYkCgYEAzCP3zixxrNqi2jEElGf7G/HrdUlijGK1iLjT
+lPbEJOYn/zW5EugsPLAZ1HNjlemVu6Mn/9xRJ2FxSgxqI68O6KpAyDrofAVcHJa
A/CIJh79IzgF+HZPD5kIZjFPVJSJwBuF8EeOLKwKcHjrg2rSS/2RYDVc3O9Kw9Zl
ay3RaLcCAwEAAaNTMFEwHQYDVR0OBBYEFDon7fNMok0ITgq9WIyjjG40UpSPMB8G
A1UdIwQYMBaAFDon7fNMok0ITgq9WIyjjG40UpSPMA8GA1UdEwEB/wQFMAMBAf8w
DQYJKoZIhvcNAQELBQADgYEAlOZGRyFoUCkRzvWROGOQsE4xjyX5frwtAXJP3+kd
Ku96mO4f+n2RsqpZH9SMV7uPAsIobxvV4tofAbP4FBfOYI71OyiPszRzo/4hdNUo
0knhDrgvY0nJW2/R4OJfFZ1g8ncpXzJ4FYLYLS037ROMungq3anBFtnTd6AyEZw5
Lz4=
-----END CERTIFICATE-----`)

	rsaKeyPEM = []byte(`-----BEGIN PRIVATE KEY-----
MIICeAIBADANBgkqhkiG9w0BAQEFAASCAmIwggJeAgEAAoGBAMwj984scazaotox
BJRn+xvx63VJYoxitYi40/pT2xCTmJ/81uRLoLDywGdRzY5XplbujJ//cUSdhcUo
MaiOvDuiqQMg66HwFXByWgPwiCYe/SM4Bfh2Tw+ZCGYxT1SUicAbhfBHjiysCnB4
64Nq0kv9kWA1XNzvSsPWZWst0Wi3AgMBAAECgYEAnk4OLx4cERWDUHzOtl9kRal3
FH8SIxew+xOJnwhESziKFRc3ddaICHBXcEfphcbGwYdAGhs3NSSKxfeDetklch5d
1PnjYtyr7H0zMkq5biOIN00xgRvkdJHG0RzsIlPy3/4PUq1NyNNEXtEea5gClJ4q
LBvnPS7W0Do1V0I1MyECQQDy6O+yGrXJLOLOrhMMf581zSfxGlWntMldMNOWiUzx
8YePoAp7akdmhhQRTQHx8rH2wcF9lDFh/4qBwrpLsCjpAkEA1yQwNOq1h0akYbI1
tY/FauSr368rBAJUjFwOa4S4rov9qseKMrHu/4OlL0RWI91uuSIywrAIZaGLwQcl
LygAnwJBAJfxQM3li0RVgWHK3Tt6MPqUY6Ga2W1X1oUmX5PQOoM0k5kxgJ0GM7db
sv3Hb6oKJ2u0cvW8Vs936wmT5rglbtECQQCekrTZfBoawE3PGJyP242GcU/hymnp
RZJt9jhGtYeuV867/uF05kOjn7O0OClJvB+tY3CIoVk/F6g7uXmF3XU/AkAdKCLR
oSm8+FWL0OC9C8y/S6w92FOr6Uw2Bv7H7Z0PdDs7KduCVWCJ6OOkDOoDA4LDwgPu
koALnXo03sDSuW48
-----END PRIVATE KEY-----`)

	rsaCert tls.Certificate
)

func init() {
	var err error
	rsaCert, err = tls.X509KeyPair(rsaCertPEM, rsaKeyPEM)
	if err != nil {
		panic(err)
	}
}
