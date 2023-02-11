package usage

import (
	"sync"
	"time"
)

var (
	mutex           sync.RWMutex
	usageByDeviceID = make(map[string]*Usage)
)

type Usage struct {
	CountryCode string
	Bytes       int64
	AsOf        time.Time
	TTLSeconds  int64
}

// Set sets the Usage in bytes for the given device as of the given time and known to be resetting within ttlSeconds
func Set(dev string, countryCode string, usage int64, asOf time.Time, ttlSeconds int64) {
	mutex.Lock()
	usageByDeviceID[dev] = &Usage{countryCode, usage, asOf, ttlSeconds}
	mutex.Unlock()
}

// Get gets the Usage for the given device.
func Get(dev string) *Usage {
	mutex.RLock()
	result := usageByDeviceID[dev]
	mutex.RUnlock()
	return result
}
