// Package blacklist provides a mechanism for blacklisting IP addresses that
// connect but never make it past our security filtering, either because they're
// not sending HTTP requests or sending invalid HTTP requests.
package blacklist

import (
	"fmt"
	"sync"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
	"github.com/getlantern/pcapper"

	"github.com/getlantern/http-proxy-lantern/v2/instrument"
)

const (
	// DefaultMaxIdleTime is used for MaxIdleTime if a non-positive value is
	// specified.
	DefaultMaxIdleTime = 2 * time.Minute

	// DefaultMaxConnectInterval is used for MaxConnectInterval if a non-positive
	// value is specified.
	DefaultMaxConnectInterval = 10 * time.Second

	// DefaultAllowedFailures is used for AllowedFailures if a non-positive value
	// is specified.
	DefaultAllowedFailures = 100

	// DefaultExpiration is used fro Expiration if a non-positive value is
	// specified.
	DefaultExpiration = 6 * time.Hour
)

var (
	log = golog.LoggerFor("blacklist")

	blacklistingEnabled = false // we've temporarily turned off blacklisting for safety
)

// Options is a set of options to initialize a blacklist.
type Options struct {
	// The maximum amount of time we'll wait between the start of a connection
	// and seeing a successful HTTP request before we mark the connection as
	// failed. Defaults to 2 minutes.
	MaxIdleTime time.Duration

	// Consecutive connection attempts within this interval will be treated as a
	// single attempt. Defaults to 10 seconds.
	MaxConnectInterval time.Duration

	// The number of consecutive failures allowed before an IP is blacklisted.
	// Defaults to 100.
	AllowedFailures int

	// How long an IP is allowed to remain on the blacklist.  In practice, an
	// IP may end up on the blacklist up to 1.1 * blacklistExpiration. Defaults to
	// 6 hours.
	Expiration time.Duration

	Instrument instrument.Instrument
}

func (opts *Options) applyDefaults() {
	if opts.MaxIdleTime <= 0 {
		opts.MaxIdleTime = DefaultMaxIdleTime
		log.Debugf("Defaulted MaxIdleTime to %v", opts.MaxIdleTime)
	}

	if opts.MaxConnectInterval <= 0 {
		opts.MaxConnectInterval = DefaultMaxConnectInterval
		log.Debugf("Defaulted MaxConnectInterval to %v", opts.MaxConnectInterval)
	}

	if opts.AllowedFailures <= 0 {
		opts.AllowedFailures = DefaultAllowedFailures
		log.Debugf("Defaulted AllowedFailures to %v", opts.AllowedFailures)
	}

	if opts.Expiration <= 0 {
		opts.Expiration = DefaultExpiration
		log.Debugf("Defaulted Expiration to %v", opts.Expiration)
	}
	if opts.Instrument == nil {
		opts.Instrument = instrument.NoInstrument{}
	}
}

// Blacklist is a blacklist of IPs.
type Blacklist struct {
	maxIdleTime         time.Duration
	maxConnectInterval  time.Duration
	allowedFailures     int
	blacklistExpiration time.Duration
	connections         chan string
	successes           chan string
	firstConnectionTime map[string]time.Time
	lastConnectionTime  map[string]time.Time
	failureCounts       map[string]int
	blacklist           map[string]time.Time
	instrument          instrument.Instrument
	mutex               sync.RWMutex
}

// New creates a new Blacklist with given options.
func New(opts Options) *Blacklist {
	opts.applyDefaults()

	bl := &Blacklist{
		maxIdleTime:         opts.MaxIdleTime,
		maxConnectInterval:  opts.MaxConnectInterval,
		allowedFailures:     opts.AllowedFailures,
		blacklistExpiration: opts.Expiration,
		connections:         make(chan string, 10000),
		successes:           make(chan string, 10000),
		firstConnectionTime: make(map[string]time.Time),
		lastConnectionTime:  make(map[string]time.Time),
		failureCounts:       make(map[string]int),
		blacklist:           make(map[string]time.Time),
		instrument:          opts.Instrument,
	}
	go bl.track()
	return bl
}

// Succeed records a success for the given addr, which resets the failure count
// for that IP and removes it from the blacklist.
func (bl *Blacklist) Succeed(ip string) {
	select {
	case bl.successes <- ip:
		// ip submitted as success
	default:
		_ = log.Errorf("Unable to record success from %v", ip)
	}
}

// OnConnect records an attempt to connect from the given IP. If the IP is
// blacklisted, this returns false.
func (bl *Blacklist) OnConnect(ip string) bool {
	if !blacklistingEnabled {
		bl.instrument.Blacklist(false)
		return true
	}
	bl.mutex.RLock()
	defer bl.mutex.RUnlock()
	_, blacklisted := bl.blacklist[ip]
	if blacklisted {
		log.Errorf("%v is blacklisted", ip)
		bl.instrument.Blacklist(true)
		return false
	}
	bl.instrument.Blacklist(false)
	select {
	case bl.connections <- ip:
		// ip submitted as connected
	default:
		_ = log.Errorf("Unable to record connection from %v", ip)
	}
	return true
}

func (bl *Blacklist) track() {
	idleTicker := time.NewTicker(bl.maxIdleTime)
	blacklistTicker := time.NewTicker(bl.blacklistExpiration / 10)
	for {
		select {
		case ip := <-bl.connections:
			bl.onConnection(ip)
		case ip := <-bl.successes:
			bl.onSuccess(ip)
		case <-idleTicker.C:
			bl.checkForIdlers()
		case <-blacklistTicker.C:
			bl.checkExpiration()
		}
	}
}

func (bl *Blacklist) onConnection(ip string) {
	now := time.Now()
	t, exists := bl.lastConnectionTime[ip]
	bl.lastConnectionTime[ip] = now
	if now.Sub(t) > bl.maxConnectInterval {
		bl.failureCounts[ip] = 0
		return
	}

	_, exists = bl.firstConnectionTime[ip]
	if !exists {
		bl.firstConnectionTime[ip] = now
	}
}

func (bl *Blacklist) onSuccess(ip string) {
	bl.failureCounts[ip] = 0
	delete(bl.lastConnectionTime, ip)
	delete(bl.firstConnectionTime, ip)
	bl.mutex.Lock()
	delete(bl.blacklist, ip)
	bl.mutex.Unlock()
}

func (bl *Blacklist) checkForIdlers() {
	log.Trace("Checking for idlers")
	now := time.Now()
	var blacklistAdditions []string
	for ip, t := range bl.firstConnectionTime {
		if now.Sub(t) > bl.maxIdleTime {
			msg := fmt.Sprintf("%v connected but failed to successfully send an HTTP request within %v", ip, bl.maxIdleTime)
			log.Trace(msg)
			delete(bl.firstConnectionTime, ip)
			ops.Begin("connect_without_request").Set("client_ip", ip).End()
			pcapper.Dump(ip, fmt.Sprintf("Blacklist Check: %v", msg))

			count := bl.failureCounts[ip] + 1
			bl.failureCounts[ip] = count
			if count >= bl.allowedFailures {
				ops.Begin("blacklist").Set("client_ip", ip).End()
				_ = log.Errorf("Blacklisting %v", ip)
				blacklistAdditions = append(blacklistAdditions, ip)
			}
		}
	}
	if len(blacklistAdditions) > 0 {
		bl.mutex.Lock()
		for _, ip := range blacklistAdditions {
			bl.blacklist[ip] = now
		}
		bl.mutex.Unlock()
	}
}

func (bl *Blacklist) checkExpiration() {
	now := time.Now()
	bl.mutex.Lock()
	for ip, blacklistedAt := range bl.blacklist {
		if now.Sub(blacklistedAt) > bl.blacklistExpiration {
			log.Tracef("Removing %v from blacklist", ip)
			delete(bl.blacklist, ip)
			delete(bl.failureCounts, ip)
			delete(bl.firstConnectionTime, ip)
		}
	}
	bl.mutex.Unlock()
}
