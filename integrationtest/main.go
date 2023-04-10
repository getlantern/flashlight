// Flashlight integration tester
//
// See README.md for more information
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/flashlight-integration-test/testcases"
	"github.com/getlantern/flashlight-integration-test/testparams"
	"github.com/getlantern/flashlight-integration-test/testrunner"
	"github.com/sirupsen/logrus"
)

var (
	testNameFlag        = flag.String("test", "all", "Test to run")
	availableTestParams []testparams.TestParams
	log                 = logrus.New()
)

func init() {
	availableTestParams = []testparams.TestParams{
		// This is a dummy test that always passes and doesn't run any cases.
		// It's useful for testing the test framework itself and as a template
		// for new tests.
		testparams.Test_Dummy,
		// http
		// -----
		testparams.Test_HTTP,
		// https
		// -----
		// TODO <10-04-2023, soltzen> There's an error in this test
		// testparams.Test_HTTPS,
		//
		// Shadowsocks
		// ---------------
		testparams.Test_Shadowsocks_NoMultiplex,
	}
}

func main() {
	flag.Parse()

	// Output logs to both stdout and to a file.
	//
	// XXX <10-04-2023, soltzen> Your stdout will be flooded since Flashlight
	// is notoriously verbose. Read the test logs from the file instead for a
	// cleaner output.
	// The flashlight loggers will be cleaned up in a future PR.
	log.SetLevel(logrus.DebugLevel)
	// Output logs to a file
	currentDate := time.Now().Format("20060102")
	logFile, err := os.OpenFile(
		fmt.Sprintf("integrationtest-%s.log", currentDate),
		os.O_CREATE|os.O_WRONLY,
		0666,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to open log file: %v", err))
	}
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

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
			// Info in a test case
			func(tc testcases.TestCase, msg string) {
				log.Infof("[%s:%s] %s", t.Params.Name, tc.Name, msg)
			},
			// Error in a test case
			func(tc testcases.TestCase, err error) {
				log.Errorf("[%s:%s] %s", t.Params.Name, tc.Name, err)
			},
			// General info in a test
			func(msg string) {
				log.Errorf("[%s] %s", t.Params.Name, msg)
			},
			// Fatal log in a test
			func(err error) {
				log.Errorf("FATAL ERROR IN TEST [%s]: %s", t.Params.Name, err)
				waitGroup.Done()
			},
			// Test is done
			func() {
				log.Infof("[%s] DONE", t.Params.Name)
				waitGroup.Done()
			})

		t.Run()
	}

	// Wait for all tests to finish
	waitGroup.Wait()
	log.Infof("All tests done")
}
