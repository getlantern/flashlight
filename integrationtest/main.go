package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/getlantern/flashlight-integration-test/rediswrapper"
	"github.com/getlantern/flashlight-integration-test/testsuite"
	"github.com/getlantern/golog"
	"github.com/rivo/tview"
)

var (
	InfraPathFlag = flag.String(
		"infra-path",
		"",
		"Path to the lantern_infrastructure repo",
	)
	testFlag = flag.String("test", "", "Test to run")
)

func main() {
	flag.Parse()

	if *InfraPathFlag == "" {
		flag.PrintDefaults()
		log.Fatal("Please specify the path to the lantern_infrastructure repo")
	}
	if *testFlag == "" {
		flag.PrintDefaults()
		log.Fatal("Please specify the test to run")
	}

	// Fetch Redis client and connect it to production Redis
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rdb, err := rediswrapper.MakeRedisClientFromInfra(ctx, *InfraPathFlag)
	if err != nil {
		log.Fatalf("Unable to make redis client: %v", err)
	}
	log.Printf("Connected to Redis: %v\n", rdb)

	app := tview.NewApplication()
	testSuiteBox := tview.NewTextView().
		SetBorder(true).
		SetTitle("Tests").
		SetChangedFunc(func() { app.Draw() })
	logBox := tview.NewTextView().
		SetBorder(true).
		SetTitle("Logs").
		SetChangedFunc(func() {
			app.Draw()
		})

	chanOutput := NewGologChanOutput()
	golog.SetOutput(output)
	go func() {
		// Refresh the logs whenever a new log comes in
		for logLine := range chanOutput.LogsChan {
			fmt.Fprintln(logBox, logLine)
		}
	}()

	ts, err := testsuite.NewTestSuite(
		*testFlag,
		rdb,
		&testsuite.IntegrationTestConfig{},
	)
	if err != nil {
		log.Fatalf("Unable to run NewTestSuite: %v", err)
	}
	go func() {
		for range ts.UpdateChan {
			testSuiteBox.SetText("")
			for _, test := range ts.Tests {
				isDoneStr := " "
				switch t.status {
				case TestStatusDone:
					isDoneStr = "X"
				case TestStatusRunning:
					isDoneStr = ">"
				case TestStatusNotStarted:
					isDoneStr = " "
				case TestStatusFailed:
					isDoneStr = "!"
				}
				fmt.Fprintln(
					testSuiteBox,
					fmt.Sprintf("[%s]: %s", isDoneStr, test.GetName()),
				)
			}
		}
	}()

	flex := tview.NewFlex().
		AddItem(testSuiteBox, 0, 1, false).
		AddItem(logBox, 0, 2, false).SetDirection(tview.FlexRow)
	if err := app.SetRoot(flex, true).Run(); err != nil {
		log.Fatalf("Unable to run tview app: %v", err)
	}

	// if err := ts.Run(rdb); err != nil {
	// 	log.Fatalf("Test failed: %v", err)
	// }
}
