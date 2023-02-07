package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/getlantern/flashlight-integration-test/rediswrapper"
	"github.com/getlantern/flashlight-integration-test/testsuite"
)

var (
	InfraPathFlag = flag.String(
		"infra-path",
		"",
		"Path to the lantern_infrastructure repo",
	)
	testNameFlag = flag.String("test", "", "Test to run")
)

func main() {
	flag.Parse()

	if *InfraPathFlag == "" {
		flag.PrintDefaults()
		log.Fatal("Please specify the path to the lantern_infrastructure repo")
	}
	if *testNameFlag == "" {
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

	ts, err := testsuite.NewTestSuite(
		*testNameFlag,
		rdb,
		// TODO <07-02-2023, soltzen> For now, hardcode the config
		&testsuite.IntegrationTestConfig{IsHttpProxyLanternLocal: true},
	)
	if err != nil {
		log.Fatalf("Unable to create test suite: %v", err)
	}
	if err := ts.RunTests(); err != nil {
		log.Fatalf("Test suite failed: %v", err)
	}
}
