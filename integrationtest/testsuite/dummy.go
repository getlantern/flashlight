package testsuite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/util"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/go-redis/redis/v8"
)

type Test_Dummy struct{ boilerplateTest }

func (t *Test_Dummy) GetName() string { return "dummy" }

func (t *Test_Dummy) Init(
	rdb *redis.Client,
	integrationTestConfig *IntegrationTestConfig,
) (*config.ProxyConfig, io.Closer, error) {
	logger.Debugf("Initializing test [Test_Dummy]")

	// Init local proxyConfig
	localProxyConfig := &config.ProxyConfig{
		PluggableTransport: "https",
		Addr:               "localhost:3223",
		AuthToken:          "bunnyfoofoo",
		Cert: string(
			util.MustReadFile(LocalHTTPProxyLanternTestCertFile),
		),
	}

	// Init local http-proxy-lantern Proxy (remote is not available for dummy
	// tests. We don't have a live dummy proxy, nor do we need one)
	logger.Debugf(
		"Initializing local http-proxy-lantern. Remote is not available",
	)
	p := &httpProxyLantern.Proxy{
		ProxyName:     "localtest-dummy",
		ProxyProtocol: localProxyConfig.PluggableTransport,
		Token:         localProxyConfig.AuthToken,
		HTTPS:         true,
		HTTPAddr:      localProxyConfig.Addr,
		KeyFile:       LocalHTTPProxyLanternTestKeyFile,
		CertFile:      LocalHTTPProxyLanternTestCertFile,
	}

	// Run the Proxy in a goroutine
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.ListenAndServe(ctx); err != nil &&
			!errors.Is(err, net.ErrClosed) &&
			!strings.Contains(err.Error(), "listener closed") {
			panic(
				fmt.Errorf("Unable to start httpProxyLantern server: %v", err),
			)
		}
	}()

	// Sleep for a bit to let the proxy start
	time.Sleep(2 * time.Second)

	// Return both. The caller will close the proxy and needs both
	// config.ProxyConfig **and** the httpProxyLantern.Proxy.
	return localProxyConfig, p, nil
}

func (t *Test_Dummy) Run(
	proxyConfig *config.ProxyConfig) error {
	// Do nothing in this dummy test
	logger.Debugf("Running test [Test_Dummy]")
	t.setStatus(TestStatusDone)
	return nil
}
