// Package integrationtest provides support for integration style tests that
// need a local web server and proxy server.
package integrationtest

import (
	"compress/gzip"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/golog"
	proxy "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/getlantern/quicwrapper"
	"github.com/getlantern/tlsdefaults"
	"github.com/getlantern/waitforserver"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/client"
)

const (
	Content  = "THIS IS SOME STATIC CONTENT FROM THE WEB SERVER"
	Token    = "AF325DF3432FDS"
	KeyFile  = "./proxykey.pem"
	CertFile = "./proxycert.pem"

	Etag        = "X-Lantern-Etag"
	IfNoneMatch = "X-Lantern-If-None-Match"

	obfs4SubDir = ".obfs4"

	oquicKey = "tAqXDihxfJDqyHy35k2NhImetkzKmoC7MFEELrYi6LI="

	shadowsocksSecret   = "foobarbaz"
	shadowsocksUpstream = "local"
	shadowsocksCipher   = "chacha20-ietf-poly1305"

	tlsmasqSNI          = "test.com"
	tlsmasqSuites       = "0xcca9,0x1301" // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_AES_128_GCM_SHA256
	tlsmasqMinVersion   = "0x0303"        // TLS 1.2
	tlsmasqServerSecret = "d0cd0e2e50eb2ac7cb1dc2c94d1bc8871e48369970052ff866d1e7e876e77a13246980057f70d64a2bdffb545330279f69bce5fd"
)

var (
	log               = golog.LoggerFor("testsupport")
	tlsmasqOriginAddr string
)

// Helper is a helper for running integration tests that provides its own web,
// proxy and config servers.
type Helper struct {
	protocol                      atomic.Value
	t                             *testing.T
	ConfigDir                     string
	HTTPSProxyServerAddr          string
	HTTPSUTPAddr                  string
	OBFS4ProxyServerAddr          string
	OBFS4UTPProxyServerAddr       string
	LampshadeProxyServerAddr      string
	LampshadeUTPProxyServerAddr   string
	QUICIETFProxyServerAddr       string
	OQUICProxyServerAddr          string
	WSSProxyServerAddr            string
	ShadowsocksProxyServerAddr    string
	ShadowsocksmuxProxyServerAddr string
	TLSMasqProxyServerAddr        string
	HTTPSSmuxProxyServerAddr      string
	HTTPSPsmuxProxyServerAddr     string
	HTTPServerAddr                string
	HTTPSServerAddr               string
	ConfigServerAddr              string
	tlsMasqOriginAddr             string
	listeners                     []net.Listener
}

// NewHelper prepares a new integration test helper including a web server for
// content, a proxy server and a config server that ties it all together. It
// also enables ForceProxying on the client package to make sure even localhost
// origins are served through the proxy. Make sure to close the Helper with
// Close() when finished with the test.
func NewHelper(t *testing.T, basePort int) (*Helper, error) {
	ConfigDir, err := ioutil.TempDir("", "integrationtest_helper")
	log.Debugf("ConfigDir is %v", ConfigDir)
	if err != nil {
		return nil, err
	}

	nextPort := basePort
	nextListenAddr := func() string {
		addr := fmt.Sprintf("localhost:%d", nextPort)
		nextPort++
		return addr
	}

	helper := &Helper{
		t:                             t,
		ConfigDir:                     ConfigDir,
		HTTPSProxyServerAddr:          nextListenAddr(),
		HTTPSUTPAddr:                  nextListenAddr(),
		OBFS4ProxyServerAddr:          nextListenAddr(),
		OBFS4UTPProxyServerAddr:       nextListenAddr(),
		LampshadeProxyServerAddr:      nextListenAddr(),
		LampshadeUTPProxyServerAddr:   nextListenAddr(),
		QUICIETFProxyServerAddr:       nextListenAddr(),
		OQUICProxyServerAddr:          nextListenAddr(),
		WSSProxyServerAddr:            nextListenAddr(),
		ShadowsocksProxyServerAddr:    nextListenAddr(),
		ShadowsocksmuxProxyServerAddr: nextListenAddr(),
		TLSMasqProxyServerAddr:        nextListenAddr(),
		HTTPSSmuxProxyServerAddr:      nextListenAddr(),
		HTTPSPsmuxProxyServerAddr:     nextListenAddr(),
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
	err = helper.writeConfig()
	if err != nil {
		helper.Close()
		return nil, err
	}

	return helper, nil
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
	kcpConfFile, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}

	err = json.NewEncoder(kcpConfFile).Encode(kcpConf)
	kcpConfFile.Close()
	if err != nil {
		return err
	}

	oqDefaults := quicwrapper.DefaultOQuicConfig([]byte(""))

	s1 := &proxy.Proxy{
		TestingLocal:             true,
		HTTPAddr:                 helper.HTTPSProxyServerAddr,
		HTTPMultiplexAddr:        helper.HTTPSSmuxProxyServerAddr,
		HTTPUTPAddr:              helper.HTTPSUTPAddr,
		Obfs4Addr:                helper.OBFS4ProxyServerAddr,
		Obfs4UTPAddr:             helper.OBFS4UTPProxyServerAddr,
		Obfs4Dir:                 filepath.Join(helper.ConfigDir, obfs4SubDir),
		LampshadeAddr:            helper.LampshadeProxyServerAddr,
		LampshadeUTPAddr:         helper.LampshadeUTPProxyServerAddr,
		QUICIETFAddr:             helper.QUICIETFProxyServerAddr,
		WSSAddr:                  helper.WSSProxyServerAddr,
		TLSMasqAddr:              helper.TLSMasqProxyServerAddr,
		ShadowsocksAddr:          helper.ShadowsocksProxyServerAddr,
		ShadowsocksMultiplexAddr: helper.ShadowsocksmuxProxyServerAddr,
		ShadowsocksSecret:        shadowsocksSecret,

		OQUICAddr:              helper.OQUICProxyServerAddr,
		OQUICKey:               oquicKey,
		OQUICCipher:            oqDefaults.Cipher,
		OQUICAggressivePadding: uint64(oqDefaults.AggressivePadding),
		OQUICMaxPaddingHint:    uint64(oqDefaults.MaxPaddingHint),
		OQUICMinPadded:         uint64(oqDefaults.MinPadded),

		TLSMasqSecret:     tlsmasqServerSecret,
		TLSMasqOriginAddr: helper.tlsMasqOriginAddr,

		Token:       Token,
		KeyFile:     KeyFile,
		CertFile:    CertFile,
		IdleTimeout: 30 * time.Second,
		HTTPS:       true,
	}

	// kcp server
	s2 := &proxy.Proxy{
		TestingLocal: true,
		HTTPAddr:     "127.0.0.1:0",
		KCPConf:      kcpConfFile.Name(),
		Token:        Token,
		KeyFile:      KeyFile,
		CertFile:     CertFile,
		IdleTimeout:  30 * time.Second,
		HTTPS:        false,
	}

	// psmux multiplexed http
	// smux multiplexed http
	s3 := &proxy.Proxy{
		TestingLocal:      true,
		HTTPS:             true,
		HTTPMultiplexAddr: helper.HTTPSPsmuxProxyServerAddr,
		MultiplexProtocol: "psmux",
		Token:             Token,
		KeyFile:           KeyFile,
		CertFile:          CertFile,
		IdleTimeout:       30 * time.Second,
	}

	go s1.ListenAndServe()
	go s2.ListenAndServe()

	err = waitforserver.WaitForServer("tcp", helper.HTTPSProxyServerAddr, 10*time.Second)
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

	// only launch / wait for this one after the cert is in place (can race otherwise.)
	go s3.ListenAndServe()
	err = waitforserver.WaitForServer("tcp", helper.HTTPSPsmuxProxyServerAddr, 10*time.Second)

	return err
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
			helper.writeProxyConfig(resp, req)
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
	_, err := w.Write([]byte(globalCfg))
	if err != nil {
		helper.t.Error(err)
	}
	w.Close()
}

func (helper *Helper) writeProxyConfig(resp http.ResponseWriter, req *http.Request) {
	log.Debug("Writing proxy config")
	proto := helper.protocol.Load().(string)
	cfg, err := helper.buildProxies(proto)
	if err != nil {
		helper.t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		helper.t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	etag := fmt.Sprintf("%x", sha256.Sum256(out))
	if req.Header.Get(IfNoneMatch) == etag {
		resp.WriteHeader(http.StatusNotModified)
		return
	}

	resp.Header().Set(Etag, etag)
	resp.WriteHeader(http.StatusOK)

	w := gzip.NewWriter(resp)
	_, err = w.Write(out)
	if err != nil {
		helper.t.Error(err)
	}
	w.Close()
}

func (helper *Helper) writeConfig() error {
	filename := filepath.Join(helper.ConfigDir, "proxies.yaml")
	proto := helper.protocol.Load().(string)
	cfg, err := helper.buildProxies(proto)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, out, 0644)
}

func (helper *Helper) buildProxies(proto string) (map[string]*chained.ChainedServerInfo, error) {
	protos := strings.Split(proto, ",")
	// multipath
	if len(protos) > 1 {
		proxies := make(map[string]*chained.ChainedServerInfo)
		for _, p := range protos {
			cfgs, err := helper.buildProxies(p)
			if err != nil {
				return nil, err
			}
			for name, cfg := range cfgs {
				cfg.MultipathEndpoint = "multipath-endpoint"
				proxies[name] = cfg
			}
		}
		return proxies, nil
	}
	var srv chained.ChainedServerInfo
	err := yaml.Unmarshal([]byte(proxiesTemplate), &srv)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal config %v", err)
	}

	srv.AuthToken = Token
	if proto == "obfs4" || proto == "utpobfs4" {
		if proto == "utpobfs4" {
			srv.Addr = helper.OBFS4UTPProxyServerAddr
		} else {
			srv.Addr = helper.OBFS4ProxyServerAddr
		}
		srv.PluggableTransport = proto
		srv.PluggableTransportSettings = map[string]string{
			"iat-mode": "0",
		}

		bridgelineFile, err2 := ioutil.ReadFile(filepath.Join(filepath.Join(helper.ConfigDir, obfs4SubDir), "obfs4_bridgeline.txt"))
		if err2 != nil {
			return nil, fmt.Errorf("Could not read obfs4_bridgeline.txt: %v", err2)
		}
		obfs4extract := regexp.MustCompile(".+cert=([^\\s]+).+")
		srv.Cert = string(obfs4extract.FindSubmatch(bridgelineFile)[1])
	} else {
		cert, err2 := ioutil.ReadFile(CertFile)
		if err2 != nil {
			return nil, fmt.Errorf("Could not read cert %v", err2)
		}
		srv.Cert = string(cert)
		if proto == "lampshade" {
			srv.Addr = helper.LampshadeProxyServerAddr
			srv.PluggableTransport = "lampshade"
		} else if proto == "quic_ietf" {
			srv.Addr = helper.QUICIETFProxyServerAddr
			srv.PluggableTransport = "quic_ietf"
		} else if proto == "oquic" {
			srv.Addr = helper.OQUICProxyServerAddr
			srv.PluggableTransport = "oquic"
			srv.PluggableTransportSettings = map[string]string{
				"oquic_key": oquicKey,
			}
		} else if proto == "wss" {
			srv.Addr = helper.WSSProxyServerAddr
			srv.PluggableTransport = "wss"
			srv.PluggableTransportSettings = map[string]string{
				"url":         fmt.Sprintf("https://%s", helper.WSSProxyServerAddr),
				"multiplexed": "true",
			}
		} else if proto == "utphttps" {
			srv.Addr = helper.HTTPSUTPAddr
			srv.PluggableTransport = "utphttps"
		} else if proto == "utplampshade" {
			srv.Addr = helper.LampshadeUTPProxyServerAddr
			srv.PluggableTransport = "utplampshade"
		} else if proto == "tlsmasq" {
			srv.Addr = helper.TLSMasqProxyServerAddr
			srv.PluggableTransport = "tlsmasq"
			srv.PluggableTransportSettings = map[string]string{
				"tlsmasq_sni":           tlsmasqSNI,
				"tlsmasq_suites":        tlsmasqSuites,
				"tlsmasq_tlsminversion": tlsmasqMinVersion,
				"tlsmasq_secret":        tlsmasqServerSecret,
			}
		} else if proto == "shadowsocks" {
			srv.Addr = helper.ShadowsocksProxyServerAddr
			srv.PluggableTransport = "shadowsocks"
			srv.PluggableTransportSettings = map[string]string{
				"shadowsocks_secret":   shadowsocksSecret,
				"shadowsocks_upstream": shadowsocksUpstream,
				"shadowsocks_cipher":   shadowsocksCipher,
			}
		} else if proto == "shadowsocks-mux" {
			srv.Addr = "multiplexed"
			srv.MultiplexedAddr = helper.ShadowsocksmuxProxyServerAddr
			srv.PluggableTransport = "shadowsocks"
			srv.PluggableTransportSettings = map[string]string{
				"shadowsocks_secret":   shadowsocksSecret,
				"shadowsocks_upstream": shadowsocksUpstream,
				"shadowsocks_cipher":   shadowsocksCipher,
			}
		} else if proto == "https+smux" {
			srv.Addr = "multiplexed"
			srv.MultiplexedAddr = helper.HTTPSSmuxProxyServerAddr
			// the default is smux, so srv.MultiplexedProtocol is unset
		} else if proto == "https+psmux" {
			srv.Addr = "multiplexed"
			srv.MultiplexedAddr = helper.HTTPSPsmuxProxyServerAddr
			srv.MultiplexedProtocol = "psmux"
		} else {
			srv.Addr = helper.HTTPSProxyServerAddr
		}

		if proto == "kcp" {
			srv.KCPSettings = kcpConf
		}
	}
	return map[string]*chained.ChainedServerInfo{"proxy-" + proto: &srv}, nil
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
