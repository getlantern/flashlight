package testcases

import "github.com/getlantern/flashlight/balancer"

var DefaultTestCases []TestCase

func init() {
	DefaultTestCases = []TestCase{
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
}
