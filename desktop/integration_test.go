package desktop

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	bclient "github.com/getlantern/borda/client"
	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
	"github.com/getlantern/waitforserver"

	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/flashlight/goroutines"
	"github.com/getlantern/flashlight/integrationtest"
	"github.com/getlantern/flashlight/logging"

	"github.com/stretchr/testify/assert"
)

const (
	LocalProxyAddr = "localhost:18345"
	SocksProxyAddr = "localhost:18346"
)

func TestProxying(t *testing.T) {
	golog.SetPrepender(logging.Timestamped)
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
			return val.Get().(float64)
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
	config.ForceProxyConfigPollInterval = 1 * time.Second
	listenPort := 23000
	nextListenAddr := func() string {
		listenPort++
		return fmt.Sprintf("localhost:%d", listenPort)
	}
	helper, err := integrationtest.NewHelper(t, nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr(), nextListenAddr())
	if !assert.NoError(t, err) {
		return
	}
	defer helper.Close()

	// Starts the Lantern App
	a, err := startApp(t, helper)
	if !assert.NoError(t, err) {
		return
	}

	// Makes a test request
	testRequest(t, helper)

	// Switch to utphttps, wait for a new config and test request again
	helper.SetProtocol("utphttps")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Switch to obfs4, wait for a new config and test request again
	helper.SetProtocol("obfs4")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Switch to utpobfs4, wait for a new config and test request again
	helper.SetProtocol("utpobfs4")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Switch to lampshade, wait for a new config and test request again
	helper.SetProtocol("lampshade")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// utplampshade doesn't currently work for some reason
	// // Switch to utplampshade, wait for a new config and test request again
	// helper.SetProtocol("utplampshade")
	// time.Sleep(2 * time.Second)
	// testRequest(t, helper)

	// Switch to kcp, wait for a new config and test request again
	helper.SetProtocol("kcp")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Switch to quic, wait for a new config and test request again
	helper.SetProtocol("quic")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Switch to oquic, wait for a new config and test request again
	helper.SetProtocol("oquic")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Switch to wss, wait for a new config and test request again
	helper.SetProtocol("wss")
	time.Sleep(2 * time.Second)
	testRequest(t, helper)

	// Disconnected Lantern and try again
	a.Disconnect()
	testRequest(t, helper)

	// Connect Lantern and try again
	a.Connect()
	testRequest(t, helper)

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
			for _, op := range borda.FullyReportedOps {
				if op == "report_issue" || op == "sysproxy_off" || op == "sysproxy_off_force" || op == "sysproxy_clear" || op == "probe" || op == "proxy_rank" || op == "proxy_selection_stability" {
					// ignore these, as we don't do them (reliably) during the integration test
					continue
				}
				if reportedOps[op] == 0 {
					missingOps = append(missingOps, op)
				} else {
					for _, lightweightOp := range borda.LightweightOps {
						if op == lightweightOp {
							if reportedOps[op] > 100 {
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

func startApp(t *testing.T, helper *integrationtest.Helper) (*App, error) {
	configURL := "http://" + helper.ConfigServerAddr
	flags := map[string]interface{}{
		"cloudconfig":             configURL,
		"frontedconfig":           configURL,
		"addr":                    LocalProxyAddr,
		"socksaddr":               SocksProxyAddr,
		"headless":                true,
		"proxyall":                true,
		"configdir":               helper.ConfigDir,
		"initialize":              false,
		"vpn":                     false,
		"stickyconfig":            false,
		"clear-proxy-settings":    false,
		"readableconfig":          true,
		"authaddr":                common.AuthServerAddr,
		"uiaddr":                  "127.0.0.1:16823",
		"borda-report-interval":   5 * time.Minute,
		"borda-sample-percentage": 0.0, // this is 0 to disable random sampling, allowing us to test fully reported ops
		"ui-domain":               "ui.lantern.io",
		"force-traffic-log":       false,
		"tl-mtu-limit":            1500,
	}

	a := &App{
		Flags: flags,
	}
	a.Init()
	// Set a non-zero User ID to make prochecker happy
	id := settings.GetUserID()
	if id == 0 {
		settings.SetUserIDAndToken(1, "token")
	}

	go func() {
		a.Run()
		a.WaitForExit()
	}()

	return a, waitforserver.WaitForServer("tcp", LocalProxyAddr, 10*time.Second)
}

func testRequest(t *testing.T, helper *integrationtest.Helper) {
	proxyURL, _ := url.Parse("http://" + LocalProxyAddr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	doRequest(t, client, "http://"+helper.HTTPServerAddr)
	doRequest(t, client, "https://"+helper.HTTPSServerAddr)
	goroutines.PrintProfile(10)
}

func doRequest(t *testing.T, client *http.Client, url string) {
	resp, err := client.Get(url)
	if assert.NoError(t, err, "Unable to GET for "+url) {
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if assert.NoError(t, err, "Unable to read response for "+url) {
			if assert.Equal(t, http.StatusOK, resp.StatusCode, "Bad response status for "+url+": "+string(b)) {
				assert.Equal(t, integrationtest.Content, string(b))
			}
		}
	}
}
