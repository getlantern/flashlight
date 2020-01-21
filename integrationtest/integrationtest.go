// Package integrationtest provides support for integration style tests that
// need a local web server and proxy server.
package integrationtest

import (
	"compress/gzip"
	"encoding/json"
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
	proxy "github.com/getlantern/http-proxy-lantern"
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

	tlsmasqSuites       = "0xcca9" // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305
	tlsmasqMinVersion   = "0x0303" // TLS 1.2
	tlsmasqServerSecret = "d0cd0e2e50eb2ac7cb1dc2c94d1bc8871e48369970052ff866d1e7e876e77a13246980057f70d64a2bdffb545330279f69bce5fd"
)

var (
	log             = golog.LoggerFor("testsupport")
	globalCfg       []byte
	proxiesTemplate []byte
)

func init() {
	bytes, err := ioutil.ReadFile("../integrationtest/global-template.yaml")
	if err != nil {
		panic(fmt.Sprintf("Could not read global-template.yaml %v", err))
	}
	globalCfg = bytes

	bytes, err = ioutil.ReadFile("../integrationtest/proxies-template.yaml")
	if err != nil {
		panic(fmt.Sprintf("Could not read proxies-template.yaml %v", err))
	}
	proxiesTemplate = bytes

}

// Helper is a helper for running integration tests that provides its own web,
// proxy and config servers.
type Helper struct {
	protocol                    atomic.Value
	t                           *testing.T
	ConfigDir                   string
	HTTPSProxyServerAddr        string
	HTTPSUTPAddr                string
	OBFS4ProxyServerAddr        string
	OBFS4UTPProxyServerAddr     string
	LampshadeProxyServerAddr    string
	LampshadeUTPProxyServerAddr string
	QUICProxyServerAddr         string
	OQUICProxyServerAddr        string
	WSSProxyServerAddr          string
	TLSMasqProxyServerAddr      string
	HTTPServerAddr              string
	HTTPSServerAddr             string
	ConfigServerAddr            string
	listeners                   []net.Listener
}

// NewHelper prepares a new integration test helper including a web server for
// content, a proxy server and a config server that ties it all together. It
// also enables ForceProxying on the client package to make sure even localhost
// origins are served through the proxy. Make sure to close the Helper with
// Close() when finished with the test.
func NewHelper(t *testing.T, httpsAddr string, obfs4Addr string, lampshadeAddr string, quicAddr string, oquicAddr string, wssAddr string, tlsmasqAddr string, httpsUTPAddr string, obfs4UTPAddr string, lampshadeUTPAddr string) (*Helper, error) {
	ConfigDir, err := ioutil.TempDir("", "integrationtest_helper")
	log.Debugf("ConfigDir is %v", ConfigDir)
	if err != nil {
		return nil, err
	}

	helper := &Helper{
		t:                           t,
		ConfigDir:                   ConfigDir,
		HTTPSProxyServerAddr:        httpsAddr,
		HTTPSUTPAddr:                httpsUTPAddr,
		OBFS4ProxyServerAddr:        obfs4Addr,
		OBFS4UTPProxyServerAddr:     obfs4UTPAddr,
		LampshadeProxyServerAddr:    lampshadeAddr,
		LampshadeUTPProxyServerAddr: lampshadeUTPAddr,
		QUICProxyServerAddr:         quicAddr,
		OQUICProxyServerAddr:        oquicAddr,
		WSSProxyServerAddr:          wssAddr,
		TLSMasqProxyServerAddr:      tlsmasqAddr,
	}
	helper.SetProtocol("https")
	client.ForceProxying()

	// Web server serves known content for testing
	err = helper.startWebServer()
	if err != nil {
		helper.Close()
		return nil, err
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
		TestingLocal:     true,
		HTTPAddr:         helper.HTTPSProxyServerAddr,
		HTTPUTPAddr:      helper.HTTPSUTPAddr,
		Obfs4Addr:        helper.OBFS4ProxyServerAddr,
		Obfs4UTPAddr:     helper.OBFS4UTPProxyServerAddr,
		Obfs4Dir:         filepath.Join(helper.ConfigDir, obfs4SubDir),
		LampshadeAddr:    helper.LampshadeProxyServerAddr,
		LampshadeUTPAddr: helper.LampshadeUTPProxyServerAddr,
		QUICAddr:         helper.QUICProxyServerAddr,
		WSSAddr:          helper.WSSProxyServerAddr,
		TLSMasqAddr:      helper.TLSMasqProxyServerAddr,

		OQUICAddr:              helper.OQUICProxyServerAddr,
		OQUICKey:               oquicKey,
		OQUICCipher:            oqDefaults.Cipher,
		OQUICAggressivePadding: uint64(oqDefaults.AggressivePadding),
		OQUICMaxPaddingHint:    uint64(oqDefaults.MaxPaddingHint),
		OQUICMinPadded:         uint64(oqDefaults.MinPadded),

		// TODO: (Harry) tlsmasq origin address
		TLSMasqSecret: tlsmasqServerSecret,

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
	return statErr
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
	_, err := w.Write(globalCfg)
	if err != nil {
		helper.t.Error(err)
	}
	w.Close()
}

func (helper *Helper) writeProxyConfig(resp http.ResponseWriter, req *http.Request) {
	log.Debug("Writing proxy config")
	proto := helper.protocol.Load().(string)
	version := "1"
	if proto == "obfs4" {
		version = "2"
	} else if proto == "lampshade" {
		version = "3"
	} else if proto == "kcp" {
		version = "4"
	} else if proto == "quic" {
		version = "5"
	} else if proto == "wss" {
		version = "6"
	} else if proto == "utphttps" {
		version = "7"
	} else if proto == "utpobfs4" {
		version = "8"
	} else if proto == "utplampshade" {
		version = "9"
	} else if proto == "oquic" {
		version = "10"
	}

	if req.Header.Get(IfNoneMatch) == version {
		resp.WriteHeader(http.StatusNotModified)
		return
	}

	cfg, err := helper.buildProxies(proto)
	if err != nil {
		helper.t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp.Header().Set(Etag, version)
	resp.WriteHeader(http.StatusOK)

	w := gzip.NewWriter(resp)
	_, err = w.Write(cfg)
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

	return ioutil.WriteFile(filename, cfg, 0644)
}

func (helper *Helper) buildProxies(proto string) ([]byte, error) {
	cfg := make(map[string]*chained.ChainedServerInfo)
	err := yaml.Unmarshal(proxiesTemplate, cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal config %v", err)
	}

	srv := cfg["fallback-template"]
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
		} else if proto == "quic" {
			srv.Addr = helper.QUICProxyServerAddr
			srv.PluggableTransport = "quic"
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
				"multiplexed":     "true",
				"pin_certificate": "true",
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
				"tm_suites":        tlsmasqSuites,
				"tm_tlsminversion": tlsmasqMinVersion,
				"tm_serversecret":  tlsmasqServerSecret,
			}
		} else {
			srv.Addr = helper.HTTPSProxyServerAddr
		}

		if proto == "kcp" {
			srv.KCPSettings = kcpConf
		}
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal config %v", err)
	}

	return out, nil
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
