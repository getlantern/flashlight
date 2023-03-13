package testcases

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/proxyimpl"
)

var DefaultTimeoutPerTestCase = 3 * time.Second

type TestCase struct {
	Name                     string
	connectionType           string
	testURL                  string
	expectedStringInResponse string
}

// Run runs a single test case.
// For each test case, it:
//  1. Creates a new dialer with the given configDir and proxyConfig
//  2. If prefixes is not nil, make sure we successfully receive all of them
//  3. Launch an HTTP request to cas.testURL with the created dialer and make
//     sure it succeeds
func (cas TestCase) Run(
	ctx context.Context,
	testName string,
	proxyConfig *config.ProxyConfig,
	configDir string,
) error {
	// Init the dialer
	dialer, err := chained.CreateDialer(
		configDir,
		fmt.Sprintf("%s-%s", testName, cas.Name),
		proxyConfig,
		common.NullUserConfig{},
	)
	if err != nil {
		return fmt.Errorf("Unable to create dialer: %v", err)
	}
	defer dialer.Stop()

	// If this test case supports prefixes, init prefix success channel and a
	// fill a waitgroup with the number of prefixes. We'll decrement the
	// waitgroup as we receive successful prefixes.
	var successfulPrefixReceivedWaitGroup sync.WaitGroup
	if proxyConfig.Prefixes != nil &&
		len(proxyConfig.Prefixes) > 0 {
		for range proxyConfig.Prefixes {
			successfulPrefixReceivedWaitGroup.Add(1)
		}
		prefixImpl, ok := dialer.Implementation().(*proxyimpl.PrefixImpl)
		if !ok {
			return fmt.Errorf("Expected PrefixImpl")
		}

		prefixImpl.SetSuccessfulPrefixCallback(func(pr proxyimpl.Prefix) {
			fmt.Printf("XXXXXXXXXX SUCCESSFUL PREFIX RECEIVED: %s\n", pr)
			// fmt.Printf("TestCase %d: Successful prefix: %s\n", ID, prefix)
			successfulPrefixReceivedWaitGroup.Done()
		})
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
	// proxyimpl/*.go for info on your specific dialer.
	//
	// Also, don't add an http.Transport.Proxy since our dialer already runs a
	// CONNECT request (for https requests) and modifies the Host header (for
	// http requests), just like the http.Transport.Proxy would do. See this in
	// action in chained/dialer.go. Our dialer knows about this from the
	// supplied "connectionType" which is either balancer.NetworkConnect or
	// balancer.NetworkPersistent (both "fake" connection types, used just to
	// inform the dialer of the connection).
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
	// fmt.Printf("Response body: %s", string(b))

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
