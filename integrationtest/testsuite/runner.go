package testsuite

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
)

func runTest(
	testParams TestParams,
	timeoutPerTestCase time.Duration,
) error {
	// Init configDir in a temp dir
	configDir, err := os.MkdirTemp("", "test")
	if err != nil {
		return fmt.Errorf("Unable to create temp dir: %v", err)
	}
	defer os.RemoveAll(configDir)

	// Run the test cases
	var testCaseWaitGroup sync.WaitGroup
	didHaveAtLeastOneError := false
	caseResultCh := make(chan TestCaseAndError, len(testParams.testCases))
	for casIdx, cas := range testParams.testCases {
		// Init the test case context
		testCaseCtx, testCaseCtxCancel := context.WithTimeout(
			context.Background(), timeoutPerTestCase)
		defer testCaseCtxCancel()

		// Run each test case in a goroutine.
		// If it fails or succeeds, capture the case and error in caseResultCh
		// channel and return.
		testCaseWaitGroup.Add(1)
		go func(casIdx int, cas TestCase) {
			if err := runTestCase(
				testCaseCtx, casIdx, cas,
				configDir,
				testParams,
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
	for i := 0; i < len(testParams.testCases); i++ {
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

	fmt.Println("----------------------------------")
	fmt.Println("----------------------------------")
	fmt.Println("----------------------------------")

	return nil
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
	testParams TestParams,
) error {
	// Init the dialer
	dialer, err := chained.CreateDialer(
		configDir,
		"test_"+strconv.Itoa(ID),
		testParams.proxyConfig,
		common.NullUserConfig{},
		chained.DialerOpts{Prefixes: testParams.prefixes},
	)
	if err != nil {
		return fmt.Errorf("Unable to create dialer: %v", err)
	}
	defer dialer.Stop()

	// If this test case supports prefixes, init prefix success channel and a
	// fill a waitgroup with the number of prefixes. We'll decrement the
	// waitgroup as we receive successful prefixes.
	var successfulPrefixReceivedWaitGroup sync.WaitGroup
	if testParams.prefixes != nil && len(testParams.prefixes) > 0 {
		for range testParams.prefixes {
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
