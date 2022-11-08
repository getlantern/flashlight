package redis

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/http-proxy-lantern/v2/usage"
	"github.com/go-redis/redis/v8"
)

const getUsageScript = `
	local clientKey = KEYS[1]

	local usage = redis.call("hmget", clientKey, "bytesIn", "bytesOut", "countryCode")
	local ttl = redis.call("ttl", clientKey)
	
	return {usage[1], usage[2], usage[3], ttl}
`

type ongoingSet struct {
	set map[string]bool
	sync.RWMutex
}

func (s *ongoingSet) add(dev string) {
	s.Lock()
	s.set[dev] = true
	s.Unlock()
}

func (s *ongoingSet) del(dev string) {
	s.Lock()
	delete(s.set, dev)
	s.Unlock()
}

func (s *ongoingSet) isMember(dev string) bool {
	s.RLock()
	_, ok := s.set[dev]
	s.RUnlock()
	return ok
}

// DeviceFetcher retrieves device information from Redis
type DeviceFetcher struct {
	rc      *redis.Client
	ongoing *ongoingSet
	queue   chan string
	ctx     context.Context
}

// NewDeviceFetcher creates a new DeviceFetcher
func NewDeviceFetcher(rc *redis.Client) *DeviceFetcher {
	df := &DeviceFetcher{
		rc:      rc,
		ongoing: &ongoingSet{set: make(map[string]bool, 512)},
		queue:   make(chan string, 512),
		ctx:     context.Background(),
	}

	go df.processDeviceUsageRequests()

	return df
}

// RequestNewDeviceUsage adds a new request for device usage to the queue
func (df *DeviceFetcher) RequestNewDeviceUsage(deviceID string) {
	if df.ongoing.isMember(deviceID) {
		return
	}
	select {
	case df.queue <- deviceID:
		df.ongoing.add(deviceID)
		// ok
	default:
		// queue full, ignore
	}
}

func (df *DeviceFetcher) processDeviceUsageRequests() {
	var scriptSHA string
	for deviceID := range df.queue {
		if scriptSHA == "" {
			var err error
			scriptSHA, err = df.rc.ScriptLoad(df.ctx, getUsageScript).Result()
			if err != nil {
				log.Errorf("Unable to load script, skip fetching usage: %v", err)
				continue
			}
		}

		if err := df.retrieveDeviceUsage(scriptSHA, deviceID); err != nil {
			log.Errorf("Error retrieving device usage: %v", err)
		}
	}
}

func (df *DeviceFetcher) retrieveDeviceUsage(scriptSHA string, deviceID string) error {
	clientKey := "_client:" + deviceID
	_vals, err := df.rc.EvalSha(df.ctx, scriptSHA, []string{clientKey}).Result()
	if err != nil {
		return err
	}
	vals := _vals.([]interface{})
	if vals[0] == nil || vals[1] == nil || vals[2] == nil || vals[3] == nil {
		// No entry found or partially stored, means no usage data so far.
		usage.Set(deviceID, "", 0, time.Now(), 0)
		return nil
	}

	_bytesIn := vals[0].(string)
	bytesIn, err := strconv.ParseInt(_bytesIn, 10, 64)
	if err != nil {
		log.Debugf("Error parsing bytesIn: %v", err)
		return nil
	}
	_bytesOut := vals[1].(string)
	bytesOut, err := strconv.ParseInt(_bytesOut, 10, 64)
	if err != nil {
		log.Debugf("Error parsing bytesOut: %v", err)
		return nil
	}
	countryCode := vals[2].(string)
	ttl := vals[3].(int64)
	usage.Set(deviceID, countryCode, bytesIn+bytesOut, time.Now(), ttl)
	df.ongoing.del(deviceID)
	return nil
}
