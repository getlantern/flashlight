package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
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
	rdb, err := makeRedisClientFromInfra(ctx, *InfraPathFlag)
	if err != nil {
		log.Fatalf("Unable to make redis client: %v", err)
	}

	if err := runTest(rdb, *testFlag); err != nil {
		log.Fatalf("Test failed: %v", err)
	}
}

func runTest(rdb *redis.Client, test string) error {
	switch test {
	case "shadowsocks":
		return testShadowsocks(rdb)
	default:
		return fmt.Errorf("Unknown test: %s", test)
	}
}
