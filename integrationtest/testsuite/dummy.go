package testsuite

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/util"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/go-redis/redis/v8"
)

type Test_Dummy struct{}

func (t *Test_Dummy) Name() string { return "dummy" }

func (t *Test_Dummy) Init(
	rdb *redis.Client,
	integrationTestConfig *IntegrationTestConfig,
) (TestParams, *httpProxyLantern.Proxy) {
	// Init TestParams
	testParams := TestParams{
		proxyConfig: &config.ProxyConfig{
			PluggableTransport: "https",
			Addr:               "localhost:3223",
			AuthToken:          "bunnyfoofoo",
			Cert:               string(util.MustReadFile(LocalHTTPProxyLanternTestCertFile)),
		},
		prefixes:  nil,
		testCases: nil,
	}

	// Init local http-proxy-lantern Proxy
	// XXX <07-02-2023, soltzen> This is mostly a copy-paste in every test,
	// but abstracting would mean this project depends even more heavily on
	// changes in http-proxy-lantern that could be hard to find since that
	// project is quite big and complex.
	p := &httpProxyLantern.Proxy{
		ProxyName:         "localtest-" + t.Name(),
		ProxyProtocol:     testParams.proxyConfig.PluggableTransport,
		Token:             testParams.proxyConfig.AuthToken,
		HTTPS:             true,
		ShadowsocksAddr:   testParams.proxyConfig.Addr,
		ShadowsocksSecret: testParams.proxyConfig.PluggableTransportSettings["shadowsocks_secret"],
		ShadowsocksCipher: testParams.proxyConfig.PluggableTransportSettings["shadowsocks_cipher"],
		KeyFile:           LocalHTTPProxyLanternTestKeyFile,
		CertFile:          LocalHTTPProxyLanternTestCertFile,
		// Below, in Run(), all the prefixes have a size of 4 bytes as
		// well.
		PrefixSize: 0,
	}

	return testParams, p
}
