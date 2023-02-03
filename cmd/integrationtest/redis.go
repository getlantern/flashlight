package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/getlantern/common/config"
	"github.com/go-redis/redis/v8"
	"gopkg.in/yaml.v3"
)

func getRedisURLFromInfra(repoPath string) string {
	log.Printf("Working with infra-path: %s\n", repoPath)
	p := path.Join(repoPath, "secret", "tools_env.env")
	file, err := os.Open(p)
	if err != nil {
		log.Fatalf("Unable to open tools_env.env in %s: %v", p, err)
	}
	defer file.Close()

	redisURL := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "REDIS_URL") {
			arr := strings.Split(scanner.Text(), "=")
			if len(arr) != 2 {
				log.Fatalf("Unexpected format for REDIS_URL: %s", scanner.Text())
			}
			redisURL = arr[1]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("Error scanning tools_env.env: %v", err)
	}
	if redisURL == "" {
		log.Fatalf("Unable to find REDIS_URL in tools_env.env")
	}
	return redisURL
}

func makeRedisClientFromInfra(ctx context.Context, infraPath string) (*redis.Client, error) {
	redisOpts, err := redis.ParseURL(getRedisURLFromInfra(ExpandPath(*InfraPathFlag)))
	if err != nil {
		return nil, fmt.Errorf("Unable to parse redis URL: %v", err)
	}
	rdb := redis.NewClient(redisOpts)

	// Check if the redis server is up.
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Error pinging redis: %v", err)
	}
	return rdb, nil
}

func RedisKey_ServersForTrack(track string) string {
	return fmt.Sprintf("%s:servers", track)
}

func fetchRandomProxyFromTrack(ctx context.Context, rdb *redis.Client, track string) (string, error) {
	proxy, err := rdb.SRandMember(ctx, RedisKey_ServersForTrack(track)).Result()
	if err != nil {
		return "", fmt.Errorf("Unable to fetch proxies from track %s: %v", track, err)
	}
	return proxy, nil
}

func fetchProxyConfig(ctx context.Context, rdb *redis.Client, proxyName string) (string, error) {
	config, err := rdb.HGet(ctx, "server->config", proxyName).Result()
	if err != nil {
		return "", fmt.Errorf("Unable to fetch proxy config for %s: %v", proxyName, err)
	}
	return config, nil
}

func fetchRandomProxyConfigFromTrack(
	ctx context.Context,
	rdb *redis.Client,
	track string) (*config.ProxyConfig, error) {
	// We're testing shadowsocks so fetch a shadowsocks track from Redis and
	// extract a random proxy from it
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proxyName, err := fetchRandomProxyFromTrack(ctx, rdb, TrackName)
	if err != nil {
		return nil, fmt.Errorf(
			"Unable to fetch random proxy from track %s: %v",
			TrackName, err)
	}
	log.Printf("Got proxy %s from track %s", proxyName, TrackName)

	proxyConfig, err := fetchProxyConfig(ctx, rdb, proxyName)
	if err != nil {
		return nil, fmt.Errorf(
			"Unable to fetch proxy config for proxy %s from track %s: %v",
			proxyName, TrackName, err)
	}
	log.Printf("Fetch proxy config for proxyName %s from trackName %s",
		proxyName, TrackName)

	// Unmarshal the proxy config
	s := new(map[string]*config.ProxyConfig)
	if err := yaml.Unmarshal([]byte(proxyConfig), &s); err != nil {
		return nil, fmt.Errorf("Unable to unmarshal config: %v", err)
	}
	// log.Printf("Unmarshaled config: %+v", s)
	// Extract the first one
	extractedProxyConfig := &config.ProxyConfig{}
	for _, v := range *s {
		extractedProxyConfig = v
		break
	}

	return extractedProxyConfig, nil
}
