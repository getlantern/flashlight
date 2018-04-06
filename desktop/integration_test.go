package desktop

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bclient "github.com/getlantern/borda/client"
	"github.com/getlantern/http-proxy-lantern"
	"github.com/getlantern/ops"
	"github.com/getlantern/tlsdefaults"
	"github.com/getlantern/waitforserver"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/geolookup"

	"github.com/stretchr/testify/assert"
)

const (
	LocalProxyAddr      = "localhost:18345"
	SocksProxyAddr      = "localhost:18346"
	ProxyServerAddr     = "localhost:18347"
	OBFS4ServerAddr     = "localhost:18348"
	LampshadeServerAddr = "localhost:18349"

	Content  = "THIS IS SOME STATIC CONTENT FROM THE WEB SERVER"
	Token    = "AF325DF3432FDS"
	KeyFile  = "./proxykey.pem"
	CertFile = "./proxycert.pem"

	Etag        = "X-Lantern-Etag"
	IfNoneMatch = "X-Lantern-If-None-Match"
)

var (
	protocol atomic.Value
)

func init() {
	protocol.Store("https")
}

func TestProxying(t *testing.T) {
	onGeo := geolookup.OnRefresh()

	var opsMx sync.RWMutex
	reportedOps := make(map[string]int)
	borda.BeforeSubmit = func(name string, ts time.Time, values map[string]bclient.Val, dimensionsJSON []byte) {
		dimensions := make(map[string]interface{})
		err := json.Unmarshal(dimensionsJSON, &dimensions)
		if err != nil {
			log.Errorf("Unable to unmarshal dimensions: %v", err)
			return
		}

		_op, found := dimensions["op"]
		if !found {
			return
		}
		op := _op.(string)

		getVal := func(name string) float64 {
			val := values[name]
			if val == nil {
				return 0
			}
			return val.Get()
		}

		opsMx.Lock()
		reportedOps[op] = reportedOps[op] + 1
		opsMx.Unlock()

		switch op {
		case "client_started":
			startupTime := getVal("startup_time")
			assert.True(t, startupTime > 0)
			assert.True(t, startupTime < 10)
		case "client_stopped":
			uptime := getVal("uptime")
			assert.True(t, uptime > 0)
			assert.True(t, uptime < 5000)
		case "traffic":
			sent := getVal("client_bytes_sent")
			recv := getVal("client_bytes_recv")
			assert.True(t, sent > 0)
			assert.True(t, recv > 0)
		case "catchall_fatal":
			assert.Equal(t, "test fatal error", dimensions["error"])
			assert.Equal(t, "test fatal error", dimensions["error_text"])
		case "probe":
			assert.True(t, getVal("probe_rtt") > 0)
		}
	}
	config.DefaultProxyConfigPollInterval = 100 * time.Millisecond

	// Web server serves known content for testing
	httpAddr, httpsAddr, err := startWebServer(t)
	if !assert.NoError(t, err) {
		return
	}

	// This is the remote proxy server
	err = startProxyServer(t)
	if !assert.NoError(t, err) {
		return
	}

	// This is a fake config server that serves up a config that points at our
	// testing proxy server.
	configAddr, err := startConfigServer(t)
	if !assert.NoError(t, err) {
		return
	}

	// We have to write out a config file so that Lantern doesn't try to use the
	// default config, which would go to some remote proxies that can't talk to
	// our fake config server.
	err = writeConfig()
	if !assert.NoError(t, err) {
		return
	}

	// Starts the Lantern App
	a, err := startApp(t, configAddr)
	if !assert.NoError(t, err) {
		return
	}

	// Makes a test request
	testRequest(t, httpAddr, httpsAddr)

	// Switch to obfs4, wait for a new config and test request again
	protocol.Store("obfs4")
	time.Sleep(2 * time.Second)
	testRequest(t, httpAddr, httpsAddr)

	// Switch to lampshade, wait for a new config and test request again
	protocol.Store("lampshade")
	time.Sleep(2 * time.Second)
	testRequest(t, httpAddr, httpsAddr)

	// Switch to kcp, wait for a new config and test request again
	protocol.Store("kcp")
	time.Sleep(2 * time.Second)
	testRequest(t, httpAddr, httpsAddr)

	// Disconnected Lantern and try again
	a.Disconnect()
	testRequest(t, httpAddr, httpsAddr)

	// Connect Lantern and try again
	a.Connect()
	testRequest(t, httpAddr, httpsAddr)

	// Do a fake proxybench op to make sure it gets reported
	ops.Begin("proxybench").Set("success", false).End()

	log.Fatal("test fatal error")
	log.Debug("Exiting")
	a.Exit(nil)

	select {
	case <-onGeo:
		// Look for reported ops several times over a 15 second period to give
		// system time to report everything
		var missingOps []string
		var overreportedOps []string
		for i := 0; i < 15; i++ {
			missingOps = make([]string, 0)
			opsMx.RLock()
			for _, op := range flashlight.FullyReportedOps {
				if op == "report_issue" || op == "sysproxy_off" || op == "sysproxy_off_force" || op == "sysproxy_clear" || op == "probe" || op == "proxy_rank" {
					// ignore these, as we don't do them (reliably) during the integration test
					continue
				}
				if reportedOps[op] == 0 {
					missingOps = append(missingOps, op)
				} else {
					for _, lightweightOp := range flashlight.LightweightOps {
						if op == lightweightOp {
							if reportedOps[op] > 6 {
								overreportedOps = append(overreportedOps, op)
							}
						}
					}
				}
			}
			opsMx.RUnlock()
			if len(missingOps) == 0 {
				break
			}
			time.Sleep(1 * time.Second)
		}
		for _, op := range missingOps {
			assert.Fail(t, "Fully reported op wasn't reported", op)
		}
		for _, op := range overreportedOps {
			assert.Fail(t, "Lightweight op was reported too much", "%v reported %d times", op, reportedOps[op])
		}
	case <-time.After(1 * time.Minute):
		assert.Fail(t, "Geolookup never succeeded")
	}
}

func startWebServer(t *testing.T) (string, string, error) {
	lh, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", "", fmt.Errorf("Unable to listen for HTTP connections: %v", err)
	}
	ls, err := tlsdefaults.Listen("localhost:0", "webkey.pem", "webcert.pem")
	if err != nil {
		return "", "", fmt.Errorf("Unable to listen for HTTPS connections: %v", err)
	}
	go func() {
		err := http.Serve(lh, http.HandlerFunc(serveContent))
		assert.NoError(t, err, "Unable to serve HTTP")
	}()
	go func() {
		err := http.Serve(ls, http.HandlerFunc(serveContent))
		assert.NoError(t, err, "Unable to serve HTTPS")
	}()
	return lh.Addr().String(), ls.Addr().String(), nil
}

func serveContent(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
	resp.Write([]byte(Content))
}

func startProxyServer(t *testing.T) error {
	kcpConfFile, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}

	err = json.NewEncoder(kcpConfFile).Encode(kcpConf)
	kcpConfFile.Close()
	if err != nil {
		return err
	}

	s1 := &proxy.Proxy{
		TestingLocal:  true,
		Addr:          ProxyServerAddr,
		Obfs4Addr:     OBFS4ServerAddr,
		Obfs4Dir:      ".",
		LampshadeAddr: LampshadeServerAddr,
		Token:         Token,
		KeyFile:       KeyFile,
		CertFile:      CertFile,
		IdleTimeout:   30 * time.Second,
		HTTPS:         true,
	}

	// kcp server
	s2 := &proxy.Proxy{
		TestingLocal: true,
		Addr:         "127.0.0.1:0",
		KCPConf:      kcpConfFile.Name(),
		Token:        Token,
		KeyFile:      KeyFile,
		CertFile:     CertFile,
		IdleTimeout:  30 * time.Second,
		HTTPS:        false,
	}

	go s1.ListenAndServe()
	go s2.ListenAndServe()

	err = waitforserver.WaitForServer("tcp", ProxyServerAddr, 10*time.Second)
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

func startConfigServer(t *testing.T) (string, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", fmt.Errorf("Unable to listen for config server connection: %v", err)
	}
	go func() {
		err := http.Serve(l, http.HandlerFunc(serveConfig(t)))
		assert.NoError(t, err, "Unable to serve config")
	}()
	return l.Addr().String(), nil
}

func serveConfig(t *testing.T) func(http.ResponseWriter, *http.Request) {
	return func(resp http.ResponseWriter, req *http.Request) {
		log.Debugf("Reading request path: %v", req.URL.String())
		if strings.Contains(req.URL.String(), "global") {
			writeGlobalConfig(t, resp, req)
		} else if strings.Contains(req.URL.String(), "prox") {
			writeProxyConfig(t, resp, req)
		} else {
			log.Errorf("Not requesting global or proxies in %v", req.URL.String())
			resp.WriteHeader(http.StatusBadRequest)
		}
	}
}

func writeGlobalConfig(t *testing.T, resp http.ResponseWriter, req *http.Request) {
	log.Debug("Writing global config")
	version := "1"

	if req.Header.Get(IfNoneMatch) == version {
		resp.WriteHeader(http.StatusNotModified)
		return
	}

	cfg, err := buildGlobal()
	if err != nil {
		t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp.Header().Set(Etag, version)
	resp.WriteHeader(http.StatusOK)

	w := gzip.NewWriter(resp)
	w.Write(cfg)
	w.Close()
}

func writeProxyConfig(t *testing.T, resp http.ResponseWriter, req *http.Request) {
	log.Debug("Writing proxy config")
	proto := protocol.Load().(string)
	version := "1"
	if proto == "obfs4" {
		version = "2"
	} else if proto == "lampshade" {
		version = "3"
	} else if proto == "kcp" {
		version = "4"
	}

	if req.Header.Get(IfNoneMatch) == version {
		resp.WriteHeader(http.StatusNotModified)
		return
	}

	cfg, err := buildProxies(proto)
	if err != nil {
		t.Error(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp.Header().Set(Etag, version)
	resp.WriteHeader(http.StatusOK)

	w := gzip.NewWriter(resp)
	w.Write(cfg)
	w.Close()
}

func writeConfig() error {
	filename := "proxies.yaml"
	err := os.Remove(filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Unable to delete existing yaml config: %v", err)
	}

	proto := protocol.Load().(string)
	cfg, err := buildProxies(proto)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, cfg, 0644)
}

func buildProxies(proto string) ([]byte, error) {
	bytes, err := ioutil.ReadFile("./proxies-template.yaml")
	if err != nil {
		return nil, fmt.Errorf("Could not read config %v", err)
	}

	cfg := make(map[string]*chained.ChainedServerInfo)
	err = yaml.Unmarshal(bytes, cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal config %v", err)
	}

	srv := cfg["fallback-template"]
	srv.AuthToken = Token
	if proto == "obfs4" {
		srv.Addr = OBFS4ServerAddr
		srv.PluggableTransport = "obfs4"
		srv.PluggableTransportSettings = map[string]string{
			"iat-mode": "0",
		}

		bridgelineFile, err2 := ioutil.ReadFile("obfs4_bridgeline.txt")
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
			srv.Addr = LampshadeServerAddr
			srv.PluggableTransport = "lampshade"
		} else {
			srv.Addr = ProxyServerAddr
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

func buildGlobal() ([]byte, error) {
	bytes, err := ioutil.ReadFile("./global-template.yaml")
	if err != nil {
		return nil, fmt.Errorf("Could not read config %v", err)
	}

	cfg := &config.Global{}
	err = yaml.Unmarshal(bytes, cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal config %v", err)
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal config %v", err)
	}

	return out, nil
}

func startApp(t *testing.T, configAddr string) (*App, error) {
	configURL := "http://" + configAddr
	flags := map[string]interface{}{
		"cloudconfig":             configURL,
		"frontedconfig":           configURL,
		"addr":                    LocalProxyAddr,
		"socksaddr":               SocksProxyAddr,
		"headless":                true,
		"proxyall":                true,
		"configdir":               ".",
		"stickyconfig":            false,
		"clear-proxy-settings":    false,
		"readableconfig":          true,
		"uiaddr":                  "127.0.0.1:16823",
		"borda-report-interval":   5 * time.Minute,
		"borda-sample-percentage": 0.0, // this is 0 to disable random sampling, allowing us to test fully reported ops
		"ui-domain":               "ui.lantern.io",
	}

	a := &App{
		ShowUI: false,
		Flags:  flags,
	}
	a.Init()
	// Set a non-zero User ID to make prochecker happy
	id := settings.GetUserID()
	if id == 0 {
		settings.SetUserID(1)
	}

	go func() {
		a.Run()
		a.WaitForExit()
	}()

	return a, waitforserver.WaitForServer("tcp", LocalProxyAddr, 10*time.Second)
}

func testRequest(t *testing.T, httpAddr string, httpsAddr string) {
	proxyURL, _ := url.Parse("http://" + LocalProxyAddr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	doRequest(t, client, "http://"+httpAddr)
	doRequest(t, client, "https://"+httpsAddr)
}

func doRequest(t *testing.T, client *http.Client, url string) {
	resp, err := client.Get(url)
	if assert.NoError(t, err, "Unable to GET for "+url) {
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if assert.NoError(t, err, "Unable to read response for "+url) {
			if assert.Equal(t, http.StatusOK, resp.StatusCode, "Bad response status for "+url+": "+string(b)) {
				assert.Equal(t, Content, string(b))
			}
		}
	}
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
