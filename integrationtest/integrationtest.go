// Package integrationtest provides support for integration style tests that
// need a local web server and proxy server.
package integrationtest

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/golog"
	hproxy "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/getlantern/tlsdefaults"
	"github.com/getlantern/waitforserver"
	"google.golang.org/protobuf/proto"

	"github.com/getlantern/flashlight/v7/apipb"
	"github.com/getlantern/flashlight/v7/client"
	userconfig "github.com/getlantern/flashlight/v7/config/user"
)

const (
	Content  = "THIS IS SOME STATIC CONTENT FROM THE WEB SERVER"
	Token    = "AF325DF3432FDS"
	KeyFile  = "./proxykey.pem"
	CertFile = "./proxycert.pem"

	Etag        = "X-Lantern-Etag"
	IfNoneMatch = "X-Lantern-If-None-Match"

	shadowsocksSecret   = "foobarbaz"
	shadowsocksUpstream = "local"
	shadowsocksCipher   = "AEAD_CHACHA20_POLY1305"

	tlsmasqSNI          = "test.com"
	tlsmasqSuites       = "0xcca9,0x1301" // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_AES_128_GCM_SHA256
	tlsmasqMinVersion   = "0x0303"        // TLS 1.2
	tlsmasqServerSecret = "d0cd0e2e50eb2ac7cb1dc2c94d1bc8871e48369970052ff866d1e7e876e77a13246980057f70d64a2bdffb545330279f69bce5fd"
)

var (
	log               = golog.LoggerFor("testsupport")
	tlsmasqOriginAddr string
	//go:embed global-cfg.yaml
	globalCfg []byte
	//go:embed proxies-template.yaml
	proxiesTemplate []byte
)

// Helper is a helper for running integration tests that provides its own web,
// proxy and config servers.
type Helper struct {
	protocol                      atomic.Value
	t                             *testing.T
	ConfigDir                     string
	BaseServerAddr                string
	HTTPSProxyServerPort          int32
	LampshadeProxyServerPort      int32
	QUICIETFProxyServerPort       int32
	WSSProxyServerPort            int32
	ShadowsocksProxyServerPort    int32
	ShadowsocksmuxProxyServerPort int32
	TLSMasqProxyServerPort        int32
	HTTPSSmuxProxyServerPort      int32
	HTTPSPsmuxProxyServerPort     int32
	HTTPServerAddr                string
	HTTPSServerAddr               string

	ConfigServerAddr  string
	tlsMasqOriginAddr string
	listeners         []net.Listener
}

// NewHelper prepares a new integration test helper including a web server for
// content, a proxy server and a config server that ties it all together. It
// also enables ForceProxying on the client package to make sure even localhost
// origins are served through the proxy. Make sure to close the Helper with
// Close() when finished with the test.
func NewHelper(t *testing.T, basePort int) (*Helper, error) {
	ConfigDir, err := os.MkdirTemp("", "integrationtest_helper")
	log.Debugf("ConfigDir is %v", ConfigDir)
	if err != nil {
		return nil, err
	}

	nextPort := int32(basePort)
	nextListenPort := func() int32 {
		nextPort++
		return nextPort
	}

	helper := &Helper{
		t:                             t,
		ConfigDir:                     ConfigDir,
		BaseServerAddr:                "localhost",
		HTTPSProxyServerPort:          nextPort,
		LampshadeProxyServerPort:      nextListenPort(),
		QUICIETFProxyServerPort:       nextListenPort(),
		WSSProxyServerPort:            nextListenPort(),
		ShadowsocksProxyServerPort:    nextListenPort(),
		ShadowsocksmuxProxyServerPort: nextListenPort(),
		TLSMasqProxyServerPort:        nextListenPort(),
		HTTPSSmuxProxyServerPort:      nextListenPort(),
		HTTPSPsmuxProxyServerPort:     nextListenPort(),
	}

	helper.SetProtocol("https")
	client.ForceProxying()

	// Web server serves known content for testing
	err = helper.startWebServer()
	if err != nil {
		helper.Close()
		return nil, err
	}

	// Start an origin server for tlsmasq to masquerade as.
	err = helper.startTLSMasqOrigin()
	if err != nil {
		helper.Close()
		return nil, fmt.Errorf("failed to start tlsmasq origin: %v", err)
	}

	// This is the remote proxy server
	err = helper.startProxyServer()
	if err != nil {
		helper.Close()
		return nil, err
	}

	// This is a fake config server that serves up a config that points at our
	// testing proxy server.
	err = helper.startConfigServer()
	if err != nil {
		helper.Close()
		return nil, err
	}

	// We have to write out a config file so that Lantern doesn't try to use the
	// default config, which would go to some remote proxies that can't talk to
	// our fake config server.
	err = helper.writeUserConfigFile()
	if err != nil {
		helper.Close()
		return nil, err
	}

	return helper, nil
}

func addDeprecatedFields(helper *Helper) {
}

func (helper *Helper) SetProtocol(protocol string) {
	helper.protocol.Store(protocol)
}

// Close closes the integration test helper and cleans up.
// TODO: actually stop the proxy (not currently supported by API)
func (helper *Helper) Close() {
	client.StopForcingProxying()
	os.RemoveAll(helper.ConfigDir)
	for _, l := range helper.listeners {
		l.Close()
	}
}

func (helper *Helper) startWebServer() error {
	lh, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("Unable to listen for HTTP connections: %v", err)
	}
	helper.listeners = append(helper.listeners, lh)
	ls, err := tlsdefaults.Listen("localhost:0", "webkey.pem", "webcert.pem")
	if err != nil {
		return fmt.Errorf("Unable to listen for HTTPS connections: %v", err)
	}
	helper.listeners = append(helper.listeners, ls)
	go func() {
		http.Serve(lh, http.HandlerFunc(serveContent))
	}()
	go func() {
		http.Serve(ls, http.HandlerFunc(serveContent))
	}()
	helper.HTTPServerAddr, helper.HTTPSServerAddr = lh.Addr().String(), ls.Addr().String()
	return nil
}

func serveContent(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
	resp.Write([]byte(Content))
}

func (helper *Helper) startProxyServer() error {
	kcpConfFile, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}

	err = json.NewEncoder(kcpConfFile).Encode(kcpConf)
	kcpConfFile.Close()
	if err != nil {
		return err
	}

	toListenAddr := func(port int32) string {
		p := strconv.Itoa(int(port))
		return helper.BaseServerAddr + ":" + p
	}

	s1 := &hproxy.Proxy{
		TestingLocal:             true,
		HTTPMultiplexAddr:        toListenAddr(helper.HTTPSProxyServerPort),
		TLSMasqAddr:              toListenAddr(helper.TLSMasqProxyServerPort),
		ShadowsocksAddr:          toListenAddr(helper.ShadowsocksProxyServerPort),
		ShadowsocksMultiplexAddr: toListenAddr(helper.ShadowsocksProxyServerPort),
		ShadowsocksSecret:        shadowsocksSecret,

		TLSMasqSecret:     tlsmasqServerSecret,
		TLSMasqOriginAddr: helper.tlsMasqOriginAddr,

		Token:       Token,
		KeyFile:     KeyFile,
		CertFile:    CertFile,
		IdleTimeout: 30 * time.Second,
		HTTPS:       true,
	}

	// kcp server
	s2 := &hproxy.Proxy{
		TestingLocal: true,
		HTTPAddr:     "127.0.0.1:0",
		KCPConf:      kcpConfFile.Name(),
		Token:        Token,
		KeyFile:      KeyFile,
		CertFile:     CertFile,
		IdleTimeout:  30 * time.Second,
		HTTPS:        false,
	}

	go s1.ListenAndServe(context.Background())
	go s2.ListenAndServe(context.Background())

	err = waitforserver.WaitForServer("tcp", toListenAddr(helper.HTTPSProxyServerPort), 10*time.Second)
	if err != nil {
		return err
	}

	// Wait for cert file to show up
	var statErr error
	for i := 0; i < 400; i++ {
		_, statErr = os.Stat(CertFile)
		if statErr != nil {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if statErr != nil {
		return statErr
	}

	return nil
}

func (helper *Helper) startConfigServer() error {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("Unable to listen for config server connection: %v", err)
	}
	helper.listeners = append(helper.listeners, l)
	go func() {
		http.Serve(l, http.HandlerFunc(helper.serveConfig()))
	}()
	helper.ConfigServerAddr = l.Addr().String()
	return nil
}

func (helper *Helper) serveConfig() func(http.ResponseWriter, *http.Request) {
	return func(resp http.ResponseWriter, req *http.Request) {
		log.Debugf("Reading request path: %v", req.URL.String())
		defer log.Debugf("Done serving request path: %v", req.URL.String())
		if strings.Contains(req.URL.String(), "global") {
			helper.writeGlobalConfig(resp, req)
		} else if strings.Contains(req.URL.String(), "prox") {
			helper.writeUserConfigResponse(resp, req)
		} else {
			log.Errorf("Not requesting global or proxies in %v", req.URL.String())
			resp.WriteHeader(http.StatusBadRequest)
		}
	}
}

func (helper *Helper) writeGlobalConfig(resp http.ResponseWriter, req *http.Request) {
	log.Debug("Writing global config")
	version := "1"

	if req.Header.Get(IfNoneMatch) == version {
		resp.WriteHeader(http.StatusNotModified)
		return
	}

	resp.Header().Set(Etag, version)
	resp.WriteHeader(http.StatusOK)

	w := gzip.NewWriter(resp)
	_, err := w.Write(globalCfg)
	if err != nil {
		helper.t.Error(err)
	}
	w.Close()
}

func (helper *Helper) writeUserConfigResponse(resp http.ResponseWriter, req *http.Request) {
	log.Debug("Writing config to response")

	protocol := helper.protocol.Load().(string)
	userCfg, err := helper.buildUserConfig([]string{protocol})
	if err != nil {
		helper.t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	out, err := proto.Marshal(userCfg)
	if err != nil {
		helper.t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	cfg, err := readConfigRequest(req)
	if err != nil {
		helper.t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check if proxy config sent by the client is different than the one we want to send
	newProxies := userCfg.GetProxy().GetProxies()
	oldProxyNames := cfg.GetProxy().Names
	if len(newProxies) != len(oldProxyNames) {
		resp.WriteHeader(http.StatusOK)
		if _, err = resp.Write(out); err != nil {
			helper.t.Error(err)
		}

		return
	}

	for i, p := range newProxies {
		if p.Name != oldProxyNames[i] {
			resp.WriteHeader(http.StatusOK)
			if _, err = resp.Write(out); err != nil {
				helper.t.Error(err)
			}

			return
		}
	}

	// if we got here, the proxies are the same
	resp.WriteHeader(http.StatusNoContent)
	if _, err = resp.Write(out); err != nil {
		helper.t.Error(err)
	}
}

func readConfigRequest(req *http.Request) (*apipb.ConfigRequest, error) {
	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	var cfg apipb.ConfigRequest
	if err = proto.Unmarshal(buf, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (helper *Helper) writeUserConfigFile() error {
	log.Debug("Writing config to file")
	protocol := helper.protocol.Load().(string)
	cfg, err := helper.buildUserConfig([]string{protocol})
	if err != nil {
		return err
	}

	out, err := proto.Marshal(cfg)
	if err != nil {
		return err
	}

	filename := filepath.Join(helper.ConfigDir, userconfig.DefaultConfigFilename)
	return os.WriteFile(filename, out, 0644)
}

func (helper *Helper) buildUserConfig(protos []string) (*apipb.ConfigResponse, error) {
	protoCfg := make([]*apipb.ProxyConnectConfig, 0, len(protos))
	for _, proto := range protos {
		p, err := helper.buildProxy(proto)
		if err != nil {
			return nil, err
		}

		protoCfg = append(protoCfg, p)
	}

	return &apipb.ConfigResponse{
		ProToken: Token,
		Country:  "CN",
		Ip:       "1.1.1.1",
		Proxy: &apipb.ConfigResponse_Proxy{
			Proxies: protoCfg,
		},
	}, nil
}

func (helper *Helper) buildProxy(proto string) (*apipb.ProxyConnectConfig, error) {
	cert, err := os.ReadFile(CertFile)
	if err != nil {
		return nil, fmt.Errorf("could not read cert %v", err)
	}

	conf := &apipb.ProxyConnectConfig{
		Name:      "AshKetchumAll",
		AuthToken: Token,
		CertPem:   cert,
		Addr:      helper.BaseServerAddr,
	}

	switch proto {
	case "https":
		conf.Port = helper.HTTPSProxyServerPort
		conf.ProtocolConfig = &apipb.ProxyConnectConfig_ConnectCfgTls{
			ConnectCfgTls: &apipb.ProxyConnectConfig_TLSConfig{
				SessionState: &apipb.ProxyConnectConfig_TLSConfig_SessionState{},
			},
		}
	case "tlsmasq":
		conf.Port = helper.TLSMasqProxyServerPort
		conf.ProtocolConfig = &apipb.ProxyConnectConfig_ConnectCfgTlsmasq{
			ConnectCfgTlsmasq: &apipb.ProxyConnectConfig_TLSMasqConfig{
				OriginAddr:               tlsmasqOriginAddr,
				Secret:                   []byte(tlsmasqServerSecret),
				TlsMinVersion:            tlsmasqMinVersion,
				TlsSupportedCipherSuites: strings.Split(tlsmasqSuites, ","),
			},
		}
		// conf.PluggableTransportSettings = map[string]string{
		// 	"tlsmasq_sni":           tlsmasqSNI,
		// }
	case "shadowsocks":
		conf.Port = helper.ShadowsocksProxyServerPort
		conf.ProtocolConfig = &apipb.ProxyConnectConfig_ConnectCfgShadowsocks{
			ConnectCfgShadowsocks: &apipb.ProxyConnectConfig_ShadowsocksConfig{
				Secret: shadowsocksSecret,
				Cipher: shadowsocksCipher,
			},
		}
	default:
		return nil, fmt.Errorf("unsupported proxy protocol %v", proto)
	}

	return conf, nil
}

func (helper *Helper) startTLSMasqOrigin() error {
	// Self-signed cert, common name: test.com
	var (
		certPem = []byte(`-----BEGIN CERTIFICATE-----
MIIC/jCCAeYCCQCfzdJ86xOcUjANBgkqhkiG9w0BAQsFADBBMRYwFAYDVQQKDA1J
bm5vdmF0ZSBMYWJzMRQwEgYDVQQLDAtFbmdpbmVlcmluZzERMA8GA1UEAwwIdGVz
dC5jb20wHhcNMjAwMTIyMTYyODQ5WhcNMzAwMTE5MTYyODQ5WjBBMRYwFAYDVQQK
DA1Jbm5vdmF0ZSBMYWJzMRQwEgYDVQQLDAtFbmdpbmVlcmluZzERMA8GA1UEAwwI
dGVzdC5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCo45LvJ5dS
2Cx6WtfNuCb5IN5fR3dq9NZF9l4lToV39sWfSA9P87+yDvl5644qwHR5QTADNBc2
ZP8D/2RYC4jb3piVsx7D+ylJ2ZWyi9YPzLJOYK4USp9bLwGB92upi5ahOBMkeXJG
4W+DJ2O+IDlz23cr/CBazlwxr3aq17uT4I1OfR9xqWqRBmh9BdXLzhxX4naynPYc
KLecnp/LZgR7xAVG9KjHIVpLOcx+85xeDf2JLIGf3RfRLKVL90UzXRuGdSjwm3b/
mX60r3kRz0iZO+DEWIt8QIsX31NUcSUyerC/2DrQDQikW4ingL2r16qhHyrg6Tr0
LK4LrZW23weZAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAJwlrX5omzSKdH1PWPWC
BfiH8CdK4lypoLH3PCPcRm6W5PvMW6qZWIN6fzrf/wn0p6wzo3LB+P4AXW6aduOj
oOGXOeYjgqLN+xP5iEZAqrmFgDOJLUO0f465rsPY69LoCoGWXKnPZ2vqasGbfXF6
KPKXirGGkAE41isKSLh1V6tyWSNWZYgcKccITyMMm75CDcZcChwApfZicw/NMU0W
O44i+Ht6wTAa0UJd9fnezE5FjJTqZg1n15HhhUb83ymxEmcoGUfiJ/PYcQSXDE3E
9mXD1VLPCzTX0QIQqo5McdHa385UokQya4BneK4MfpkHa8lUAYwWceGL02XgxFF1
/GE=
-----END CERTIFICATE-----`)
		keyPem = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCo45LvJ5dS2Cx6
WtfNuCb5IN5fR3dq9NZF9l4lToV39sWfSA9P87+yDvl5644qwHR5QTADNBc2ZP8D
/2RYC4jb3piVsx7D+ylJ2ZWyi9YPzLJOYK4USp9bLwGB92upi5ahOBMkeXJG4W+D
J2O+IDlz23cr/CBazlwxr3aq17uT4I1OfR9xqWqRBmh9BdXLzhxX4naynPYcKLec
np/LZgR7xAVG9KjHIVpLOcx+85xeDf2JLIGf3RfRLKVL90UzXRuGdSjwm3b/mX60
r3kRz0iZO+DEWIt8QIsX31NUcSUyerC/2DrQDQikW4ingL2r16qhHyrg6Tr0LK4L
rZW23weZAgMBAAECggEAS+5tTFrXfSa18JjRN6uY0h9F+z5tYUgM4k2fDFTeSw5G
0ZMbV032nL6AyaDvPSdj9nQpevc7jHgh85Eqcy9Ua84LehqbNW/Bo3NRC4I1Tssw
S27KNVNLjDp5Cg7Md+DLa1aDvL1hdJ68fRIDlSJ10jIUxVDI1yq6ZphF2Q+/RP9Q
W4U306ckXJq3w74lSXrQ00i4FER8B2FDZihmpclmN42tvgJBjVR792ZSEldg7Pa+
0QY07UM7sGX12dSmmRPSof690wNRg55slYq3YQ6NBClsiiNqj2sS3rYkBMvh07BM
BAQL1ybFeHyILcrq5H3FOupLK/FJErCDc8HhaO26lQKBgQDWFMMrbk2TP2Ih5DUL
96i1KLdcIGpVrnMYe9k5fGGSXpqQUEvagmYFLxFMW5grYgGxkJFGt/pkpKCzcyQk
kpOZhn5H4t6/WpjkJT3cO+KzNa5EHd0rvPSq2le9pEJSTU7dTRGkclrGpbeWUhH0
Re7+hswAwSPuM7XHIGX3sh7njwKBgQDJ9XdfNc2a8Ac7ZMy2nQ2IohwJAOYbQYBA
XrqaECYLFIApzdDakx33xdrmxpAAyjcHEQC6GRptSeqDsK43R+BaD7r3YnkBS6uI
+/dncIGsd6mmzVGjXpldORk4TGqTi0XFj8+4UWxUQ45H7YLwA49/jVXvczwnv6j4
qH13OiFKVwKBgQDGR9yssTEwnJgrg86OEwgzIk8SCQPz7+uyVaNQVx+YDf9igrx+
2h/b1UhUTNGX/OJMr/WeZnCIHuKo0pA7P3dtzt/PfRWKbkMFrGirPtwt2B5cAL0E
8bI7PJffke/Lgsb0uZkJktD5BCwSElmGwe8l13vDhx/cVBCdKijHTjbJiQKBgCu7
7E3B6PRUZjyGZ45kFDoyYL/SYgIk/RDzcpVKSfK8TcS/vSqYETVGs1CmTyjcoW32
UKH8LazdBNvfttphxkO6hFJuEKYnLM5NQhY0VuBySVrFu5gVNEDrzHpUkf/BeSp/
KgxQFZVpy7XnySMQolKM2L8xxSUWbBDs676V5/+hAoGAJDpng/G6J4WSaL0ilaLM
gnvrmkW4QsmF+ijROgFOy+I8RpWlmMJkwJXRCj9N9ZC6rqaMSqahxVBmajOter4g
rk3unSMy6rlHWxyOpqStPWjnZlNz+1R7a6dBD+L2tY5jNJCGL8L7PejgAZWd/uwK
o91tzH1xsfoYsMnt6AP4cIQ=
-----END PRIVATE KEY-----`)
	)

	decodeUint16 := func(s string) (uint16, error) {
		b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint16(b), nil
	}

	// We need to ensure the negotiated cipher suite and TLS version are acceptable to the dialer.
	suites := []uint16{}
	suiteStrings := strings.Split(tlsmasqSuites, ",")
	if len(suiteStrings) == 0 {
		return errors.New("no cipher suites specified")
	}
	for _, s := range suiteStrings {
		suite, err := decodeUint16(s)
		if err != nil {
			return fmt.Errorf("bad cipher string '%s': %v", s, err)
		}
		suites = append(suites, suite)
	}
	versStr := tlsmasqMinVersion
	minVersion, err := decodeUint16(versStr)
	if err != nil {
		return fmt.Errorf("bad TLS version string '%s': %v", versStr, err)
	}

	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %v", err)
	}
	l, err := tls.Listen("tcp", "", &tls.Config{
		Certificates: []tls.Certificate{cert},
		CipherSuites: suites,
		MinVersion:   minVersion,
	})
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil && strings.Contains(err.Error(), "use of closed network connection") {
				// Unexported, but stable error: https://golang.org/src/internal/poll/fd.go#L16
				return
			}
			if err != nil {
				log.Debugf("tlsmasq origin server: accept failure: %v", err)
				continue
			}
			go func(c net.Conn) {
				// Force the handshake so that it can be proxied.
				if err := c.(*tls.Conn).Handshake(); err != nil {
					log.Debugf("tlsmasq origin server: handshake failure: %v", err)
					return
				}
			}(conn)
		}
	}()

	helper.tlsMasqOriginAddr = l.Addr().String()
	helper.listeners = append(helper.listeners, l)
	return nil
}

var kcpConf = map[string]interface{}{
	"scavengettl": 600,
	"datashard":   10,
	"interval":    50,
	"mtu":         1350,
	"sockbuf":     4194304,
	"parityshard": 3,
	"sndwnd":      128,
	"mode":        "fast2",
	"crypt":       "salsa20",
	"key":         "thisisreallyakey",
	"snmpperiod":  60,
	"rcvwnd":      512,
	"conn":        1,
	"keepalive":   10,
	"listen":      "127.0.0.1:8975",
	"remoteaddr":  "127.0.0.1:8975",
}
