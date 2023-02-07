package testsuite

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/flashlight/balancer"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/go-redis/redis/v8"
)

// const TrackName = "dofra1u16ru-https-pro"

type Test_Shadowsocks_NoMultiplex_NoPrefix struct{ boilerplateTest }

func (t *Test_Shadowsocks_NoMultiplex_NoPrefix) GetName() string {
	return "shadowsocks-no-multiplex-no-prefix"
}

func (t *Test_Shadowsocks_NoMultiplex_NoPrefix) Init(
	rdb *redis.Client,
	integrationTestConfig *IntegrationTestConfig,
) (*config.ProxyConfig, io.Closer, error) {
	// Init http-proxy-lantern from remote, if specified
	//if !integrationTestConfig.IsHttpProxyLanternLocal {
	//	// Taken from here:
	//	// https://github.com/getlantern/lantern-infrastructure/blob/ed3d4b11fb11c6a19a92c45a7fcf8b521e7352cb/etc/current_production_track_config.py#L175
	//	// TODO <06-02-2023, soltzen> Technically, this **WILL NOT** work since that track:
	//	// - expects no prefixes
	//	// - uses multiplexing and this client does not
	//	//
	//	// I'll keep it here as an example
	//	return initRemoteHttpProxyLantern(rdb, "ir-anfra-ss-dnsovertcp")
	//}

	// Init local proxyConfig
	logger.Debugf("Initializing local http-proxy-lantern. Remote is not available")
	localProxyConfig := &config.ProxyConfig{
		PluggableTransport: "shadowsocks",
		PluggableTransportSettings: map[string]string{
			"shadowsocks_secret": "secret",
			"shadowsocks_cipher": "chacha20-ietf-poly1305",
		},
		Addr:      "localhost:3223",
		AuthToken: "bunnyfoofoo",
		Cert:      string(util.MustReadFile(LocalHTTPProxyLanternTestCertFile)),
	}

	// Init local http-proxy-lantern Proxy
	p := &httpProxyLantern.Proxy{
		ProxyName:         "localtest-shadowsocks-no-multiplex-no-prefix",
		ProxyProtocol:     localProxyConfig.PluggableTransport,
		Token:             localProxyConfig.AuthToken,
		HTTPS:             true,
		ShadowsocksAddr:   localProxyConfig.Addr,
		ShadowsocksSecret: localProxyConfig.PluggableTransportSettings["shadowsocks_secret"],
		ShadowsocksCipher: localProxyConfig.PluggableTransportSettings["shadowsocks_cipher"],
		KeyFile:           LocalHTTPProxyLanternTestKeyFile,
		CertFile:          LocalHTTPProxyLanternTestCertFile,
		// Below, in Run(), all the prefixes have a size of 4 bytes as
		// well.
		PrefixSize: 0,
	}

	// Run the Proxy in a goroutine
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.ListenAndServe(ctx); err != nil {
			panic(
				fmt.Errorf("Unable to start httpProxyLantern server: %v", err),
			)
		}
	}()

	// Return both. The caller will close the proxy and needs both
	// config.ProxyConfig **and** the httpProxyLantern.Proxy.
	return localProxyConfig, p, nil
}

func (t *Test_Shadowsocks_NoMultiplex_NoPrefix) Run(
	proxyConfig *config.ProxyConfig) error {
	configDir, err := os.MkdirTemp("", "test")
	if err != nil {
		return fmt.Errorf("Unable to create temp dir: %v", err)
	}
	defer os.RemoveAll(configDir)

	// Init the test cases
	cases := []TestCase{
		// XXX <27-01-2023, soltzen> Whichever domain you test with **must be**
		// in the non-throttle list in http-proxy-lantern. Otherwise, the
		// connection will be throttled (read: blocked) and the test will fail.
		// Why the connection is blocked and not throttled (i.e., rate limited)
		// is hyper-weird and wrong. I'm looking into it.
		// The non-throttle list:
		// https://github.com/getlantern/http-proxy-lantern/blob/58d8f6f84a0b82065830adec15aa0f88638936dd/domains/domains.go#L62
		//
		//
		// {
		// 	connectionType:           balancer.NetworkConnect,
		// 	testURL:                  "https://www.google.com/humans.txt",
		// 	expectedStringInResponse: "Google is built by a large team of engineers",
		// },
		{
			connectionType:           balancer.NetworkConnect,
			testURL:                  "https://lantern.io",
			expectedStringInResponse: "open internet",
		},
		{
			connectionType:           balancer.NetworkConnect,
			testURL:                  "https://stripe.com/de",
			expectedStringInResponse: "Online-Bezahldienst",
		},
		{
			connectionType:           balancer.NetworkConnect,
			testURL:                  "https://www.paymentwall.com",
			expectedStringInResponse: "paymentwall",
		},
	}

	// Run the test cases
	var testCaseWaitGroup sync.WaitGroup
	didHaveAtLeastOneError := false
	caseResultCh := make(chan TestCaseAndError, len(cases))
	for casIdx, cas := range cases {
		// Init the test case context
		testCaseCtx, testCaseCtxCancel := context.WithTimeout(
			context.Background(), 5*time.Second)
		defer testCaseCtxCancel()

		// Run each test case in a goroutine.
		// If it fails or succeeds, capture the case and error in caseResultCh
		// channel and return.
		testCaseWaitGroup.Add(1)
		go func(casIdx int, cas TestCase) {
			if err := runTestCase(
				testCaseCtx, casIdx, cas,
				configDir, proxyConfig,
				nil, // prefixes
			); err != nil {
				caseResultCh <- TestCaseAndError{
					cas,
					fmt.Errorf("Test case %v failed: %v", casIdx, err)}
			} else {
				caseResultCh <- TestCaseAndError{cas, nil}
			}
			testCaseWaitGroup.Done()
		}(casIdx, cas)
	}

	// Wait for all test cases to finish or timeout
	testCaseWaitGroup.Wait()
	fmt.Println("----------------------------------")
	fmt.Println("----------------------------------")
	fmt.Println("----------------------------------")

	// Print the results
	for i := 0; i < len(cases); i++ {
		casErr := <-caseResultCh
		if casErr.err != nil {
			fmt.Printf(
				"XXXXXXXXXX TEST CASE %v FAILED: %s\n",
				casErr.TestCase,
				casErr.err,
			)
			didHaveAtLeastOneError = true
		} else {
			fmt.Printf("XXXXXXXXXX TEST CASE %v PASSED\n", casErr.TestCase)
		}
	}

	// Make sure the reader knows at least one test case failed, if any
	if didHaveAtLeastOneError {
		return fmt.Errorf("AT LEAST ONE TEST CASE FAILED\n")
	}

	return nil
}
