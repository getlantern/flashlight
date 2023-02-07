package testsuite

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/caarlos0/log"
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/projectpath"
	"github.com/getlantern/flashlight-integration-test/rediswrapper"
	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/golog"
	"github.com/go-redis/redis/v8"
)

var logger = golog.LoggerFor("integrationtest")
var LocalHTTPProxyLanternTestKeyFile = filepath.Join(
	projectpath.Root,
	"testdata",
	"httpproxylantern-test-key.pem")
var LocalHTTPProxyLanternTestCertFile = filepath.Join(
	projectpath.Root,
	"testdata",
	"httpproxylantern-test-cert.pem")
var allTests map[string]Test
var updateChan chan struct{}

func init() {
	allTests = map[string]Test{
		// Dummy
		// -----
		// This is a dummy test that always passes. It's useful for testing
		// the test framework itself and as a template for new tests.
		"dummy": &Test_Dummy{},
		//
		// Shadowsocks
		// ---------------
		// "shadowsocks-nomultiplex-singleprefix": &Test_Shadowsocks_NoMultiplex_SinglePrefix{},
		"shadowsocks-nomultiplex-noprefix":       &Test_Shadowsocks_NoMultiplex_NoPrefix{},
		"shadowsocks-nomultiplex-multipleprefix": &Test_Shadowsocks_NoMultiplex_MultiplePrefix{},
	}
	updateChan = make(chan struct{})
}

// Run runs the test specified by testName, or "all" to run all tests.
// func Run(
// 	rdb *redis.Client,
// 	testName string,
// 	integrationTestConfig *IntegrationTestConfig,
// ) error {
// 	// Init
// 	proxyConfig, httpProxyLanternHandle, err := t.Init(
// 		rdb, integrationTestConfig)
// 	if err != nil {
// 		return fmt.Errorf("Unable to init test %s: %s", testName, err)
// 	}
// 	defer httpProxyLanternHandle.Close()

// 	// Run
// 	return t.Run(proxyConfig)
// }

type TestCase struct {
	connectionType           string
	testURL                  string
	expectedStringInResponse string
}

type TestCaseAndError struct {
	TestCase
	err error
}

type IntegrationTestConfig struct {
	IsHttpProxyLanternLocal bool
}

type TestStatus int

const (
	TestStatusNotStarted TestStatus = 0
	TestStatusRunning    TestStatus = iota
	TestStatusDone       TestStatus = iota
	TestStatusFailed     TestStatus = iota
)

type Test interface {
	Init(*redis.Client, *IntegrationTestConfig) (
		proxyConfig *config.ProxyConfig,
		httpProxyLanternHandle io.Closer,
		err error)
	Run(*config.ProxyConfig) error
	GetName() string
}

func initRemoteHttpProxyLantern(
	rdb *redis.Client,
	remoteTestTrackName string,
) (*config.ProxyConfig, io.Closer, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second)
	defer cancel()
	proxyConfig, err := rediswrapper.FetchRandomProxyConfigFromTrack(
		ctx, rdb, remoteTestTrackName)
	if err != nil {
		return nil, nil,
			fmt.Errorf(
				"Unable to fetch random proxy from track %s: %v",
				remoteTestTrackName, err)
	}
	httpProxyLanternHandle := util.IoNopCloser{}

	return proxyConfig, httpProxyLanternHandle, nil
}

// runTestCase runs a single test case.
// For each test case, it:
//  1. Creates a new dialer with the given configDir and proxyConfig
//  2. If prefixes is not nil, make sure we successfully receive all of them
//  3. Launch an HTTP request to cas.testURL with the created dialer and make
//     sure it succeeds
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

	// If this test case supports prefixes, init prefix success channel and a
	// fill a waitgroup with the number of prefixes. We'll decrement the
	// waitgroup as we receive successful prefixes.
	var successfulPrefixReceivedWaitGroup sync.WaitGroup
	if prefixes != nil && len(prefixes) > 0 {
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
	}

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

	// Wait for all prefixes to be successful.
	// This only applies to test cases that use prefixes.
	// For ones that don't, this is a no-op.
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

type TestSuite struct {
	Tests      []Test
	cfg        *IntegrationTestConfig
	rdb        *redis.Client
	UpdateChan chan struct{}
}

func NewTestSuite(
	testName string,
	rdb *redis.Client,
	cfg *IntegrationTestConfig,
) (*TestSuite, error) {
	testsToRun := []Test{}

	testName = strings.ToLower(testName)
	if testName == "all" {
		// Add all tests
		for _, test := range allTests {
			testsToRun = append(testsToRun, test)
		}
	} else {
		// Add specific test
		t, ok := allTests[testName]
		if !ok {
			return nil, fmt.Errorf("Test %s not found", testName)
		}
		testsToRun = append(testsToRun, t)
	}

	return &TestSuite{
		Tests:      testsToRun,
		cfg:        cfg,
		rdb:        rdb,
		UpdateChan: make(chan struct{}),
	}, nil
}

func (ts *TestSuite) RunTests() error {
	for _, t := range ts.Tests {
		log.Debugf("Selected test: %s", t.GetName())
		proxyConfig, httpProxyLanternHandle, err := t.Init(
			ts.rdb,
			ts.cfg,
		)
		if err != nil {
			return fmt.Errorf("Unable to init test %s: %s", t.GetName(), err)
		}
		defer httpProxyLanternHandle.Close()

		if err := t.Run(proxyConfig); err != nil {
			return fmt.Errorf("Test %s failed: %s", t.GetName(), err)
		}
	}
	return nil
}
