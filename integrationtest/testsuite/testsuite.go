package testsuite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/projectpath"
	"github.com/getlantern/flashlight-integration-test/rediswrapper"
	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/golog"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
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
		"shadowsocks-no-multiplex-no-prefix":       &Test_Shadowsocks_NoMultiplex_NoPrefix{},
		"shadowsocks-no-multiplex-multiple-prefix": &Test_Shadowsocks_NoMultiplex_MultiplePrefix{},
	}
}

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

type TestParams struct {
	proxyConfig *config.ProxyConfig
	prefixes    [][]byte
	testCases   []TestCase
}

type TestStatus int

const (
	TestStatusNotStarted TestStatus = 0
	TestStatusRunning    TestStatus = iota
	TestStatusDone       TestStatus = iota
	TestStatusFailed     TestStatus = iota
)

type Test interface {
	Init(rdb *redis.Client,
		integrationTestConfig *IntegrationTestConfig,
	) (TestParams, *httpProxyLantern.Proxy)
	Name() string
}

type TestSuite struct {
	tests []Test
	cfg   *IntegrationTestConfig
	rdb   *redis.Client
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
		tests: testsToRun,
		cfg:   cfg,
		rdb:   rdb,
	}, nil
}

func (ts *TestSuite) RunTests() error {
	for _, t := range ts.tests {
		logger.Debugf("Selected test: %s", t.Name())
		testParams, httpProxyLanternHandle := t.Init(
			ts.rdb,
			ts.cfg,
		)

		// Start httpProxyLantern server
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		go initLocalHttpProxyLantern(ctx, httpProxyLanternHandle)

		// Sleep for a bit to let the proxy start
		time.Sleep(2 * time.Second)

		if err := runTest(testParams, 10*time.Second); err != nil {
			return fmt.Errorf("Test %s failed: %s", t.Name(), err)
		}

		httpProxyLanternHandle.Close()
	}
	return nil
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

func initLocalHttpProxyLantern(ctx context.Context, p *httpProxyLantern.Proxy) {
	if err := p.ListenAndServe(ctx); err != nil {
		switch {
		// Ignore closed network errors
		case errors.Is(err, net.ErrClosed):
		case strings.Contains(err.Error(), "listener closed"):
			break
		default:
			panic(
				logger.Errorf(
					"Unable to start httpProxyLantern server: %v",
					err,
				),
			)
		}
	}
}
