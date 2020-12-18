package ios

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	tun2socks "github.com/eycorsican/go-tun2socks/core"

	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/dnsgrab/persistentcache"
	"github.com/getlantern/errors"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/bandwidth"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
)

const (
	maxDNSGrabCache = 1000 // this doesn't need to be huge because our fake DNS records have a TTL of only 1 second
	dnsCacheMaxAge  = 1 * time.Hour

	quotaSaveInterval            = 1 * time.Minute
	shortFrontedAvailableTimeout = 30 * time.Second
	longFrontedAvailableTimeout  = 5 * time.Minute

	trackMemoryInterval = 5 * time.Second
	forceGCInterval     = 25 * time.Millisecond

	dialTimeout        = 30 * time.Second
	closeTimeout       = 1 * time.Second
	maxConcurrentDials = 4
)

type Writer interface {
	Write([]byte) bool
}

type writerAdapter struct {
	client *client
}

func (wa *writerAdapter) Write(b []byte) (int, error) {
	// // drop if memory gets low
	// if !wa.client.memcap.allowed(len(b)) {
	// 	atomic.AddInt64(&wa.client.droppedPacketsSent, 1)
	// 	return 0, nil
	// }
	atomic.AddInt64(&wa.client.packetsSent, 1)

	ok := wa.client.packetsOut.Write(b)
	if !ok {
		return 0, errors.New("error writing")
	}
	return len(b), nil
}

type ClientWriter interface {
	// Write writes the given bytes. As a side effect of writing, we periodically
	// record updated bandwidth quota information in the configured quota.txt file.
	// If user has exceeded bandwidth allowance, returns a positive integer
	// representing the bandwidth allowance.
	Write([]byte) (int, error)

	// Reconfigure forces the ClientWriter to update its configuration
	Reconfigure()

	Close() error
}

type cw struct {
	ipStack        io.WriteCloser
	client         *client
	bal            *balancer.Balancer
	quotaTextPath  string
	lastSavedQuota time.Time
}

func (c *cw) Write(b []byte) (int, error) {
	// // drop if memory gets low
	// if !c.client.memcap.allowed(len(b)) {
	// 	atomic.AddInt64(&c.client.droppedPacketsRecv, 1)
	// 	return 0, nil
	// }
	atomic.AddInt64(&c.client.packetsRecv, 1)

	_, err := c.ipStack.Write(b)

	result := 0
	if time.Since(c.lastSavedQuota) > quotaSaveInterval {
		c.lastSavedQuota = time.Now()

		quota, tracked := bandwidth.GetQuota()
		// Only save if quota has actually been tracked already
		if tracked {
			if quota != nil && quota.MiBUsed > quota.MiBAllowed {
				result = int(quota.MiBAllowed)
			}

			go func() {
				if quota == nil {
					log.Debug("Clearing bandwidth quota file")
					writeErr := ioutil.WriteFile(c.quotaTextPath, []byte{}, 0644)
					if writeErr != nil {
						log.Errorf("Unable to clear quota file: %v", writeErr)
					}
				} else {
					log.Debugf("Saving bandwidth quota file with %d/%d", quota.MiBUsed, quota.MiBAllowed)
					writeErr := ioutil.WriteFile(c.quotaTextPath, []byte(fmt.Sprintf("%d/%d", quota.MiBUsed, quota.MiBAllowed)), 0644)
					if writeErr != nil {
						log.Errorf("Unable to write quota file: %v", writeErr)
					}
				}
			}()
		}
	}

	return result, err
}

func (c *cw) Reconfigure() {
	dialers, err := c.client.loadDialers()
	if err != nil {
		// this causes the NetworkExtension process to die. Since the VPN is configured as "on-demand",
		// the OS will automatically restart the service, at which point we'll read the new config anyway.
		panic(log.Errorf("Unable to load dialers on reconfigure: %v", err))
	}

	c.bal.Reset(dialers)
}

func (c *cw) Close() error {
	c.bal.Close()
	return nil
}

type client struct {
	packetsOut         Writer
	udpDialer          UDPDialer
	memChecker         MemChecker
	configDir          string
	mtu                int
	capturedDNSHost    string
	realDNSHost        string
	memcap             *memCapper
	uc                 *UserConfig
	tcpHandler         *proxiedTCPHandler
	udpHandler         *directUDPHandler
	ipStack            tun2socks.LWIPStack
	clientWriter       *cw
	started            time.Time
	packetsRecv        int64
	droppedPacketsRecv int64
	packetsSent        int64
	droppedPacketsSent int64
}

func Client(packetsOut Writer, udpDialer UDPDialer, memChecker MemChecker, configDir string, mtu int, capturedDNSHost, realDNSHost string) (ClientWriter, error) {
	// Lock to the current thread and limit the number of OS threads we use to keep memory usage down
	// Each OS thread has a stack of 512KB, which adds up quickly.
	runtime.LockOSThread()
	runtime.GOMAXPROCS(1)

	if mtu <= 0 {
		log.Debug("Defaulting MTU to 1500")
		mtu = 1500
	}

	c := &client{
		packetsOut:      packetsOut,
		udpDialer:       udpDialer,
		memChecker:      memChecker,
		configDir:       configDir,
		mtu:             mtu,
		capturedDNSHost: capturedDNSHost,
		realDNSHost:     realDNSHost,
		memcap:          newMemCapper(memChecker.BytesBeforeCritical()),
		started:         time.Now(),
	}
	go c.trackMemory()

	return c.start()
}

func (c *client) start() (ClientWriter, error) {
	if err := c.loadUserConfig(); err != nil {
		return nil, log.Errorf("error loading user config: %v", err)
	}

	log.Debugf("Running client for device '%v' at config path '%v'", c.uc.GetDeviceID(), c.configDir)
	log.Debugf("Max buffer bytes: %d", buffers.MaxBufferBytes())

	dialers, err := c.loadDialers()
	if err != nil {
		return nil, err
	}
	bal := balancer.New(func() bool { return c.uc.AllowProbes }, 30*time.Second, dialers...)

	cacheFile := filepath.Join(c.configDir, "dnsgrab.cache")
	cache, err := persistentcache.New(cacheFile, dnsCacheMaxAge)
	if err != nil {
		return nil, errors.New("Unable to initialize dnsgrab cache at %v: %v", cacheFile, err)
	}
	grabber, err := dnsgrab.ListenWithCache(
		"127.0.0.1:0",
		c.realDNSHost,
		cache,
	)
	if err != nil {
		return nil, errors.New("Unable to start dnsgrab: %v", err)
	}

	c.tcpHandler = newProxiedTCPHandler(c, bal, grabber)
	c.udpHandler = newDirectUDPHandler(c, c.udpDialer, grabber, c.capturedDNSHost)

	ipStack := tun2socks.NewLWIPStack()
	wa := &writerAdapter{c}
	tun2socks.RegisterOutputFn(wa.Write)
	tun2socks.RegisterTCPConnHandler(c.tcpHandler)
	tun2socks.RegisterUDPConnHandler(c.udpHandler)

	freeMemory()

	c.clientWriter = &cw{
		ipStack:       ipStack,
		client:        c,
		bal:           bal,
		quotaTextPath: filepath.Join(c.configDir, "quota.txt"),
	}

	return c.clientWriter, nil
}

func (c *client) loadUserConfig() error {
	cf := &configurer{configFolderPath: c.configDir}
	uc, err := cf.readUserConfig()
	if err != nil {
		return err
	}
	c.uc = uc
	return nil
}

func (c *client) loadDialers() ([]balancer.Dialer, error) {
	cf := &configurer{configFolderPath: c.configDir}
	chained.PersistSessionStates(c.configDir)

	proxies := make(map[string]*chained.ChainedServerInfo)
	_, _, err := cf.openConfig(proxiesYaml, proxies, []byte{})
	if err != nil {
		return nil, err
	}

	dialers := chained.CreateDialers(c.configDir, proxies, c.uc)
	chained.TrackStatsFor(dialers, c.configDir, false)
	return dialers, nil
}

func partialUserConfigFor(deviceID string) *UserConfig {
	return userConfigFor(0, "", deviceID)
}

func userConfigFor(userID int, proToken, deviceID string) *UserConfig {
	return &UserConfig{
		UserConfigData: *common.NewUserConfigData(
			deviceID,
			int64(userID),
			proToken,
			nil, // Headers currently unused
			"",  // Language currently unused
		),
	}
}
