package testrunner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/testparams"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
)

type Test struct {
	infoCallback  func(testcases.TestCase, string)
	errCallback   func(testcases.TestCase, error)
	fatalCallback func(error)
	doneCallback  func()
	Params        testparams.TestParams
}

func NewTest(params testparams.TestParams) *Test {
	return &Test{
		Params: params,
	}
}

func (l *Test) SetLogCallbacks(
	infoCallback func(testcases.TestCase, string),
	errCallback func(testcases.TestCase, error),
	fatalCallback func(error),
	doneCallback func(),
) {
	l.infoCallback = infoCallback
	l.errCallback = errCallback
	l.fatalCallback = fatalCallback
	l.doneCallback = doneCallback
}

func (l *Test) Info(tc testcases.TestCase, s string) {
	if l.infoCallback != nil {
		l.infoCallback(tc, s)
	}
}

func (l *Test) Error(tc testcases.TestCase, err error) {
	if l.errCallback != nil {
		l.errCallback(tc, err)
	}
}

func (l *Test) Fatal(err error) {
	if l.fatalCallback != nil {
		l.fatalCallback(err)
	}
}

func (l *Test) Done() {
	if l.doneCallback != nil {
		l.doneCallback()
	}
}

func (t *Test) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proxyHandle, err := initHttpProxyLantern(
		t.Params.Name,
		t.Params.ProxyConfig,
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	isProxyReady := make(chan struct{})
	go func() {
		if err := proxyHandle.ListenAndServe(
			ctx,
			func() { close(isProxyReady) },
		); err != nil {
			switch {
			// Ignore closed network errors
			case errors.Is(err, net.ErrClosed):
			case strings.Contains(err.Error(), "listener closed"):
				break
			default:
				panic(
					fmt.Errorf(
						"Unable to start httpProxyLantern server: %v",
						err,
					),
				)
			}
		}
	}()
	defer proxyHandle.Close()
	<-isProxyReady

	fmt.Printf(
		"[%s] Proxy is ready: Running %d test cases...\n",
		t.Params.Name,
		len(t.Params.TestCases),
	)

	// Init configDir in a temp dir
	configDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
		return
	}
	defer os.RemoveAll(configDir)

	// Run the test cases serially (for now. Easier to debug)
	// atLeastOneFailed := false
	for _, cas := range t.Params.TestCases {
		// Init the test case context
		testCaseCtx, testCaseCtxCancel := context.WithTimeout(
			context.Background(), testcases.DefaultTimeoutPerTestCase)
		defer testCaseCtxCancel()

		if err := cas.Run(testCaseCtx, t.Params.Name, t.Params.ProxyConfig, configDir); err != nil {
			t.Fatal(fmt.Errorf("Test case %s failed: %v", cas.Name, err))
			return
			// atLeastOneFailed = true
			// t.Error(cas, err)
		}
		t.Info(cas, "OK")
	}

	// Make sure the reader knows at least one test case failed, if any
	// if atLeastOneFailed {
	// 	t.Fatal(errors.New("at least one test case failed"))
	// 	return
	// }

	t.Done()
}

func initHttpProxyLantern(
	testName string,
	cfg *config.ProxyConfig,
) (*httpProxyLantern.Proxy, error) {
	// Mostly takes a params.ProxyConfig and returns an httpProxyLantern.Proxy

	p := &httpProxyLantern.Proxy{
		// Default values that work for all cases
		Pro:                     true,
		ThrottleRefreshInterval: 5 * time.Minute,
		DiffServTOS:             0,
		IdleTimeout:             90 * time.Second,
		// General
		ProxyName:     "proxy-" + testName,
		ProxyProtocol: cfg.PluggableTransport,
		Token:         cfg.AuthToken,
		CertFile:      testparams.LocalHTTPProxyLanternTestCertFile,
		KeyFile:       testparams.LocalHTTPProxyLanternTestKeyFile,
	}

	if cfg.Prefixes != nil && len(cfg.Prefixes) > 0 {
		// TODO <01-04-2023, soltzen> For now, we're only supporting one prefix size.
		// Later, we'll make it so that http-proxy-lantern will take an array
		// of sizes and try each one until one succeeds.
		//
		// Divide by 2 since each prefix is a string of hex characters so
		// "41414141" is 4 bytes representing "AAAA"
		p.PrefixSize = len(cfg.Prefixes[0]) / 2
	}

	switch cfg.PluggableTransport {
	case "", "http", "https", "utphttp", "utphttps":
		p.HTTPAddr = cfg.Addr
		p.HTTPMultiplexAddr = cfg.MultiplexedAddr
		// enhttp is mostly used for domain-fronting so you can ignore it for
		// testing
		// ENHTTPAddr                         string
		// ENHTTPServerURL                    string
		// ENHTTPReapIdleTime                 time.Duration
	case "shadowsocks":
		p.ShadowsocksAddr = cfg.Addr
		p.ShadowsocksMultiplexAddr = cfg.MultiplexedAddr
		p.ShadowsocksSecret = cfg.PluggableTransportSettings["shadowsocks_secret"]
		p.ShadowsocksCipher = cfg.PluggableTransportSettings["shadowsocks_cipher"]
		//
		// unused
		// - p.ShadowsocksReplayHistory
	}
	// obfs4
	// -----
	// Obfs4Addr                          string
	// Obfs4MultiplexAddr                 string
	// Obfs4Dir                           string
	// Obfs4HandshakeConcurrency          int
	// Obfs4MaxPendingHandshakesPerClient int
	// Obfs4HandshakeTimeout              time.Duration
	//
	// kcp
	// -----
	// KCPConf                            string
	//
	// quic
	// ----
	// QUICIETFAddr                       string
	// QUICUseBBR                         bool
	//
	// wss
	// ---
	// WSSAddr                            string
	//
	// TLS
	// ---
	// TLSListenerAllowTLS13              bool
	// TLSMasqAddr                        string
	// TLSMasqOriginAddr                  string
	// TLSMasqSecret                      string
	// TLSMasqTLSMinVersion               uint16
	// TLSMasqTLSCipherSuites             []uint16
	//
	//
	// Multiplex stuff
	// -------
	// MultiplexProtocol             string
	// SmuxVersion                   int
	// SmuxMaxFrameSize              int
	// SmuxMaxReceiveBuffer          int
	// SmuxMaxStreamBuffer           int
	// PsmuxVersion                  int
	// PsmuxMaxFrameSize             int
	// PsmuxMaxReceiveBuffer         int
	// PsmuxMaxStreamBuffer          int
	// PsmuxDisablePadding           bool
	// PsmuxMaxPaddingRatio          float64
	// PsmuxMaxPaddedSize            int
	// PsmuxDisableAggressivePadding bool
	// PsmuxAggressivePadding        int
	// PsmuxAggressivePaddingRatio   float64
	//
	// Useless here
	// -----
	// TestingLocal                       bool
	// HoneycombSampleRate                int
	// TeleportSampleRate                 int
	// PromExporterAddr                   string
	// CountryLookup                      geo.CountryLookup
	// ISPLookup                          geo.ISPLookup
	// CfgSvrAuthToken                    string
	// CfgSvrCacheClear                   time.Duration
	// ConnectOKWaitsForUpstream          bool
	// EnableMultipath                    bool
	// EnableReports                      bool
	// ProxiedSitesSamplePercentage       float64
	// ProxiedSitesTrackingID             string
	// ReportingRedisClient               *rclient.Client
	// VersionCheck                       bool
	// VersionCheckRange                  string
	// VersionCheckRedirectURL            string
	// VersionCheckRedirectPercentage     float64
	// GoogleSearchRegex                  string
	// GoogleCaptchaRegex                 string
	// BlacklistMaxIdleTime               time.Duration
	// BlacklistMaxConnectInterval        time.Duration
	// BlacklistAllowedFailures           int
	// BlacklistExpiration                time.Duration
	// BuildType                          string
	// BBRUpstreamProbeURL                string
	// PacketForwardAddr                  string
	// ExternalIntf                       string
	// SessionTicketKeyFile               string
	// FirstSessionTicketKey              string
	// RequireSessionTickets              bool
	// MissingTicketReaction              tlslistener.HandshakeReaction
	// ExternalIP                         string
	// TunnelPorts                        string

	return p, nil
}
