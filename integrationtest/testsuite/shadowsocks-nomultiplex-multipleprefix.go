package testsuite

import (
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight-integration-test/util"
	"github.com/getlantern/flashlight/balancer"
	httpProxyLantern "github.com/getlantern/http-proxy-lantern/v2"
	"github.com/go-redis/redis/v8"
)

type Test_Shadowsocks_NoMultiplex_MultiplePrefix struct{}

func (t *Test_Shadowsocks_NoMultiplex_MultiplePrefix) Name() string {
	return "shadowsocks-no-multiplex-multiple-prefix"
}

func (t *Test_Shadowsocks_NoMultiplex_MultiplePrefix) Init(
	rdb *redis.Client,
	integrationTestConfig *IntegrationTestConfig,
) (TestParams, *httpProxyLantern.Proxy) {
	// Init TestParams
	testParams := TestParams{
		proxyConfig: &config.ProxyConfig{
			PluggableTransport: "shadowsocks",
			PluggableTransportSettings: map[string]string{
				"shadowsocks_secret": "secret",
				"shadowsocks_cipher": "chacha20-ietf-poly1305",
			},
			Addr:      "localhost:3223",
			AuthToken: "bunnyfoofoo",
			Cert: string(
				util.MustReadFile(LocalHTTPProxyLanternTestCertFile),
			),
		},
		prefixes: [][]byte{
			// Not necessary, but for this test, we want to make sure all
			// prefixes are the same length since we're hardcoding the
			// prefix size in the http-proxy-lantern proxy below.
			[]byte("AAAA"),
			[]byte("BBBB"),
			[]byte("CCCC"),
		},
		testCases: []TestCase{
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
		},
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
		// See testParams. We're assuming all prefixes are the same length.
		PrefixSize: len(testParams.prefixes[0]),
	}

	return testParams, p
}
