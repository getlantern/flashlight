package tlsutil

import (
	"crypto/tls"
	"fmt"
	"testing"
)

// TestOverAllSuites runs a test function over all combinations of suite and version supported by
// this package. As of Go 1.15, this includes every suite and version supported by crypto/tls.
func TestOverAllSuites(t *testing.T, testFn func(t *testing.T, version, suite uint16)) {
	t.Helper()

	pre13Suites, tls13Suites := []uint16{}, []uint16{}
	for suiteValue, suite := range cipherSuites {
		if _, is13 := suite.(cipherSuiteTLS13); is13 {
			tls13Suites = append(tls13Suites, suiteValue)
		} else {
			pre13Suites = append(pre13Suites, suiteValue)
		}
	}

	makeTest := func(version, suite uint16) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			testFn(t, version, suite)
		}
	}

	for _, version := range []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12} {
		for _, suite := range pre13Suites {
			t.Run(fmt.Sprintf("version_%#x_suite%s", version, tls.CipherSuiteName(suite)), makeTest(version, suite))
		}
	}
	for _, suite := range tls13Suites {
		t.Run(fmt.Sprintf("version_%#x_suite%s", tls.VersionTLS13, tls.CipherSuiteName(suite)), makeTest(tls.VersionTLS13, suite))
	}
}
