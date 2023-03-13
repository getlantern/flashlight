package main

import (
	"flag"
	"strings"
	"sync"

	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/testparams"
	"github.com/getlantern/flashlight-integration-test/testrunner"
	log "github.com/sirupsen/logrus"
)

var (
	testNameFlag        = flag.String("test", "all", "Test to run")
	availableTestParams []testparams.TestParams
)

func init() {
	availableTestParams = []testparams.TestParams{
		// This is a dummy test that always passes and doesn't run any cases.
		// It's useful for testing the test framework itself and as a template
		// for new tests.
		testparams.Test_Dummy,
		// http
		// -----
		testparams.Test_HTTP_NoPrefix,
		testparams.Test_HTTP_MultiplePrefix,
		// https
		// -----
		testparams.Test_HTTPS_NoPrefix,
		//
		// Shadowsocks
		// ---------------
		testparams.Test_Shadowsocks_NoMultiplex_NoPrefix,
		testparams.Test_Shadowsocks_NoMultiplex_MultiplePrefix,
	}
}

func main() {
	flag.Parse()

	// Init logging
	// log.SetReportCaller(true)

	// Run all tests if no test name is specified
	// Or, run the specified test
	testName := strings.ToLower(*testNameFlag)
	tests := []*testrunner.Test{}
	for _, tp := range availableTestParams {
		if testName == "all" || tp.Name == testName {
			tests = append(tests, testrunner.NewTest(tp))
		}
	}

	// Set loggers for each test
	waitGroup := sync.WaitGroup{}
	for _, t := range tests {
		waitGroup.Add(1)
		t.SetLogCallbacks(
			// Info
			func(tc testcases.TestCase, msg string) {
				log.Infof("TESTCASE [%s:%s]: %s", t.Params.Name, tc.Name, msg)
			},
			// Error
			func(tc testcases.TestCase, err error) {
				log.Errorf("TESTCASE [%s:%s]: %s", t.Params.Name, tc.Name, err)
			},
			// Fatal
			func(err error) {
				log.Errorf("FATAL ERROR IN TEST [%s]: %s", t.Params.Name, err)
				waitGroup.Done()
			},
			// Done
			func() {
				log.Infof("TEST [%s] DONE", t.Params.Name)
				waitGroup.Done()
			})

		go t.Run()
	}

	// Wait for all tests to finish
	waitGroup.Wait()
	log.Infof("All tests done")
}
