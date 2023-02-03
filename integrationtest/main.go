package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/getlantern/flashlight-integration-test/rediswrapper"
	"github.com/getlantern/flashlight-integration-test/tests"
)

var (
	InfraPathFlag = flag.String("infra-path", "", "Path to the lantern_infrastructure repo")
	testFlag      = flag.String("test", "", "Test to run")
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

	if err := tests.Run(rdb, *testFlag, &tests.IntegrationTestConfig{}); err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}
