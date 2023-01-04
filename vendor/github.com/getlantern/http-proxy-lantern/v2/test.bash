#! /bin/bash

TEST_REDIS_CONTAINER=http-proxy-lantern-test-redis

function fail() {
    echo "$1"
    exit 1
}

function tearDown() {
    echo "Shutting down local test Redis:"
    docker stop $TEST_REDIS_CONTAINER
}

function printRedisLogs() {
    if [ "$1" == "true" ]; then
        echo "Test Redis logs:"
        docker logs $TEST_REDIS_CONTAINER
    fi
}

trap tearDown EXIT

echo "Starting local test Redis. Container ID:"
docker run \
  --name $TEST_REDIS_CONTAINER \
  -p 6379:6379 \
  -v "$PWD"/test/test-redis-data:/opt/getlantern/ \
  -e ALLOW_EMPTY_PASSWORD=yes \
  --rm -d bitnami/redis:latest || fail "Failed to start local Redis server"

go test ./... || (printRedisLogs "$1"; exit 1)