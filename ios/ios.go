package ios

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/packetforward"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/bandwidth"
	"github.com/getlantern/flashlight/buffers"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"

	"github.com/dustin/go-humanize"
)

const (
	memLimitInMiB                = 12
	memLimitInBytes              = memLimitInMiB * 1024 * 1024
	quotaSaveInterval            = 1 * time.Minute
	shortFrontedAvailableTimeout = 30 * time.Second
	longFrontedAvailableTimeout  = 5 * time.Minute
)

var (
	log = golog.LoggerFor("ios")
)

type Writer interface {
	Write([]byte) bool
}

type writerAdapter struct {
	Writer
}

func (wa *writerAdapter) Write(b []byte) (int, error) {
	ok := wa.Writer.Write(b)
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
	io.Writer
	client         *client
	bal            *balancer.Balancer
	quotaTextPath  string
	lastSavedQuota time.Time
}

func (c *cw) Write(b []byte) (int, error) {
	_, err := c.Writer.Write(b)

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
	packetsOut Writer
	configDir  string
	mtu        int
	uc         *UserConfig
}

func Client(packetsOut Writer, configDir string, mtu int) (ClientWriter, error) {
	go trackMemory()
	go limitMemory()

	if mtu <= 0 {
		log.Debug("Defaulting MTU to 1500")
		mtu = 1500
	}

	c := &client{
		packetsOut: packetsOut,
		configDir:  configDir,
		mtu:        mtu, // hardcoding this to support large segments
	}

	return c.start()
}

func (c *client) start() (ClientWriter, error) {
	if err := c.loadUserConfig(); err != nil {
		return nil, err
	}

	log.Debugf("Running client for device '%v' at config path '%v'", c.uc.GetDeviceID(), c.configDir)
	log.Debugf("Max buffer bytes: %d", buffers.MaxBufferBytes())

	dialers, err := c.loadDialers()
	if err != nil {
		return nil, err
	}
	bal := balancer.New(func() bool { return c.uc.AllowProbes }, 30*time.Second, dialers...)

	w := packetforward.Client(&writerAdapter{c.packetsOut}, 30*time.Second, func(ctx context.Context) (net.Conn, error) {
		return bal.DialContext(ctx, "connect", "127.0.0.1:3000")
	})

	freeMemory()

	return &cw{
		Writer:        w,
		client:        c,
		bal:           bal,
		quotaTextPath: filepath.Join(c.configDir, "quota.txt"),
	}, nil
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

	dialers := chained.CreateDialers(proxies, c.uc)
	chained.TrackStatsFor(dialers, c.configDir, false)
	return dialers, nil
}

func trackMemory() {
	for {
		memstats := &runtime.MemStats{}
		runtime.ReadMemStats(memstats)
		log.Debugf("Memory InUse: %v    Alloc: %v    Sys: %v",
			humanize.Bytes(memstats.HeapInuse),
			humanize.Bytes(memstats.Alloc),
			humanize.Bytes(memstats.Sys))
		time.Sleep(5 * time.Second)
	}
}

func limitMemory() {
	for {
		freeMemory()
		time.Sleep(5 * time.Second)
	}
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

func freeMemory() {
	runtime.GC()
	debug.FreeOSMemory()
}
