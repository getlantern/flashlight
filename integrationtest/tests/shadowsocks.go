package tests

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/go-redis/redis/v8"
)

// Taken from here:
// https://github.com/getlantern/lantern-infrastructure/blob/ed3d4b11fb11c6a19a92c45a7fcf8b521e7352cb/etc/current_production_track_config.py#L175
// const TrackName = "dofra1u16ru-https-pro"

// TODO <03-02-2023, soltzen> Send this to http-proxy-lantern locally to init it

type Test_Shadowsocks_NoMultiplex_MultiplePrefix struct{}

func (t *Test_Shadowsocks_NoMultiplex_MultiplePrefix) Init(
	rdb *redis.Client,
	integrationTestConfig *IntegrationTestConfig,
) (*config.ProxyConfig, io.Closer, error) {
	// Init http-proxy-lantern, either local or remote
	const remoteTestTrackName = "ir-anfra-ss-dnsovertcp"
	var localProxyConfig *config.ProxyConfig = &config.ProxyConfig{}

	// Init http-proxy-lantern, either local or remote
	return initHttpProxyLanternLocalOrRemote(
		rdb,
		integrationTestConfig,
		remoteTestTrackName,
		localProxyConfig)
}

func (t *Test_Shadowsocks_NoMultiplex_MultiplePrefix) Run(proxyConfig *config.ProxyConfig) error {
	// Modify the transport
	// proxyConfig.PluggableTransport = "shadowsocks"
	// proxyConfig.Addr = "localhost:3223"
	// proxyConfig.MultiplexedAddr = "localhost:3223"
	// b, err := os.ReadFile("/Users/soltzen/dev/lantern/http-proxy-lantern/ss-track/cert.pem")
	// if err != nil {
	// 	return fmt.Errorf("Unable to read cert.pem: %v", err)
	// }
	// proxyConfig.Cert = string(b)
	// proxyConfig.AuthToken = "6zSbMrauzKETEgTNHJN92KGZeI55b2RFxPItiZ8x2Rqjjpuzg79fgeiZUEJgxn3X"
	// // proxyConfig.MultiplexedAddr = "localhost:3223"
	// proxyConfig.PluggableTransportSettings["shadowsocks_secret"] = "locUgMMrZYF5Qhon5TcvJiL9JyJ6seho4pwPiZOKHto="

	// Create the dialer
	configDir, err := os.MkdirTemp("", "test")
	if err != nil {
		return fmt.Errorf("Unable to create temp dir: %v", err)
	}
	prefixes := [][]byte{
		[]byte("AAAA"),
		[]byte("BBBB"),
		[]byte("CCCC"),
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
	caseSuccessCh := make(chan TestCaseAndError, len(cases))
	for casIdx, cas := range cases {
		// Init the test case context
		testCaseCtx, testCaseCtxCancel := context.WithTimeout(
			context.Background(), 5*time.Second)
		defer testCaseCtxCancel()

		// Run it
		testCaseWaitGroup.Add(1)
		go func(casIdx int, cas TestCase) {
			if err := runTestCase(
				testCaseCtx, casIdx, cas,
				configDir, proxyConfig, prefixes); err != nil {
				caseSuccessCh <- TestCaseAndError{
					cas,
					fmt.Errorf("Test case %v failed: %v", casIdx, err)}
			} else {
				caseSuccessCh <- TestCaseAndError{cas, nil}
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
		casErr := <-caseSuccessCh
		if casErr.err != nil {
			log.Printf(
				"XXXXXXXXXX TEST CASE %v FAILED: %s\n", casErr.TestCase, casErr.err)
			didHaveAtLeastOneError = true
		} else {
			log.Printf("XXXXXXXXXX TEST CASE %v PASSED\n", casErr.TestCase)
		}
	}

	// Make sure the reader knows at least one test case failed, if any
	if didHaveAtLeastOneError {
		return fmt.Errorf("AT LEAST ONE TEST CASE FAILED\n")
	}

	return nil
}

func runTestCase(
	ctx context.Context,
	ID int,
	cas TestCase,
	configDir string,
	proxyConfig *config.ProxyConfig,
	prefixes [][]byte,
) error {
	// Init the dialer
	dialer, err := chained.CreateDialer(
		configDir,
		"test_"+strconv.Itoa(ID),
		proxyConfig,
		common.NullUserConfig{},
		chained.DialerOpts{Prefixes: prefixes},
	)
	if err != nil {
		return fmt.Errorf("Unable to create dialer: %v", err)
	}
	defer dialer.Stop()

	// Init prefix success channel and a fill a waitgroup with the number of
	// prefixes. We'll decrement the waitgroup as we receive successful
	// prefixes.
	var successfulPrefixReceivedWaitGroup sync.WaitGroup
	for range prefixes {
		successfulPrefixReceivedWaitGroup.Add(1)
	}
	// Receive successful prefixes and decrement the waitgroup
	go func() {
		ch := dialer.SuccessfulPrefixChan()
		if ch == nil {
			panic("No successful prefix channel for test case " +
				strconv.Itoa(ID))
		}
		for prefix := range ch {
			fmt.Printf("TestCase %d: Successful prefix: %s\n", ID, prefix)
			successfulPrefixReceivedWaitGroup.Done()
		}
	}()

	// Run a test HTTP request with the dialer
	if err := runTestHTTPRequestWithDialer(
		ctx,
		dialer,
		cas.connectionType,
		cas.testURL,
		cas.expectedStringInResponse); err != nil {
		return fmt.Errorf("while running test HTTP request (%v): %v",
			cas.testURL, err)
	}

	// Wait for all prefixes to be successful
	if !util.WaitForWaitGroup(
		&successfulPrefixReceivedWaitGroup,
		5*time.Second) {
		return fmt.Errorf("Timed-out waiting for successful prefixes")
	}

	return nil
}

func runTestHTTPRequestWithDialer(
	ctx context.Context,
	dialer balancer.Dialer,
	connectionType, testURL, expectedStringInResponse string,
) error {
	// Make a test request
	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("Unable to create request: %v", err)
	}

	// Use an HTTP transport that uses the dialer.
	//
	// XXX <27-01-2023, soltzen> Don't use an http.Client since that will
	// force a TLS handshake. Our dialer already handles that (see
	// chained/*_impl.go for info on your specific dialer.
	//
	// Also, don't add an http.Transport.Proxy since our dialer already runs a
	// CONNECT request (for https requests) and modifies the Host header (for
	// http requests), just like the http.Transport.Proxy would do. See this in
	// action in chained/dialer.go. Our dialer knows about this from the
	// supplied "connectionType" which is either balancer.NetworkConnect or
	// balancer.NetworkPersistent (both "fake" connection types, used just to
	// inform the dialer of the connection).
	//
	// TODO <27-01-2023, soltzen> TBH the above "fake" connection types are a
	// complete hack. http.Transport.Proxy knows how to handle this case
	// without the dialer doing all this extra work.
	rt := &http.Transport{
		DialContext: func(
			ctx context.Context,
			network, addr string) (net.Conn, error) {
			conn, upstreamErr, err := dialer.DialContext(
				ctx, connectionType, addr)
			if err != nil {
				return nil, fmt.Errorf(
					"DialContext upstream: %v | error: %v", upstreamErr, err)
			}
			return conn, nil
		},
	}

	// Run the request
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("Unable to make request: %v", err)
	}

	// Read the response
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Unable to read response body: %v", err)
	}
	defer resp.Body.Close()
	// fmt.Printf("PINEAPPLE Response body: %s", string(b))

	// Assert response status code and body
	if resp.StatusCode != 200 {
		return fmt.Errorf("Unexpected status code: %d", resp.StatusCode)
	}
	if !strings.Contains(string(b), expectedStringInResponse) {
		return fmt.Errorf(
			"expected string [%s] was not found in response: %s",
			expectedStringInResponse, string(b))
	}
	return nil
}
