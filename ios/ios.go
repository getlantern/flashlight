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

	"github.com/getlantern/bandwidth"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/packetforward"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"

	"github.com/dustin/go-humanize"
)

const (
	memLimitInMiB     = 12
	memLimitInBytes   = memLimitInMiB * 1024 * 1024
	quotaSaveInterval = 1 * time.Minute
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

type WriteCloser interface {
	// Write writes the given bytes. As a side effect of writing, we periodically
	// record updated bandwidth quota information in the configured quota.txt file.
	// If user has exceeded bandwidth allowance, returns a positive integer
	// representing the bandwidth allowance.
	Write([]byte) (int, error)

	Close() error
}

type wc struct {
	io.Writer
	bal            *balancer.Balancer
	quotaTextPath  string
	lastSavedQuota time.Time
}

func (c *wc) Write(b []byte) (int, error) {
	_, err := c.Writer.Write(b)

	result := 0
	if time.Since(c.lastSavedQuota) > quotaSaveInterval {
		quota := bandwidth.GetQuota()
		quota = &bandwidth.Quota{
			MiBUsed:    600,
			MiBAllowed: 500,
		}
		if quota != nil {
			c.lastSavedQuota = time.Now()
			go func() {
				writeErr := ioutil.WriteFile(c.quotaTextPath, []byte(fmt.Sprintf("%d/%d", quota.MiBUsed, quota.MiBAllowed)), 0644)
				if writeErr != nil {
					log.Errorf("Unable to write quota text: %v", writeErr)
				}
			}()

			if quota.MiBUsed > quota.MiBAllowed {
				result = int(quota.MiBAllowed)
			}
		}
	}

	return result, err
}

func (c *wc) Close() error {
	c.bal.Close()
	return nil
}

type client struct {
	packetsOut Writer
	configDir  string
	mtu        int
	uc         common.UserConfig
}

func Client(packetsOut Writer, configDir string, mtu int) (WriteCloser, error) {
	go trackAndLimitMemory()

	if mtu <= 0 {
		log.Debug("Defaulting MTU to 1500")
		mtu = 1500
	}

	c := &client{
		packetsOut: packetsOut,
		configDir:  configDir,
		mtu:        mtu,
	}

	return c.start()
}

func (c *client) start() (WriteCloser, error) {
	if err := c.loadUserConfig(); err != nil {
		return nil, err
	}

	log.Debugf("Running client for device '%v' at config path '%v'", c.uc.GetDeviceID(), c.configDir)

	dialers, err := c.loadDialers()
	if err != nil {
		return nil, err
	}
	bal := balancer.New(30*time.Second, dialers...)

	w := packetforward.Client(&writerAdapter{c.packetsOut}, c.mtu, 30*time.Second, func(ctx context.Context) (net.Conn, error) {
		return bal.DialContext(ctx, "connect", "127.0.0.1:3000")
	})
	return &wc{Writer: w, bal: bal, quotaTextPath: filepath.Join(c.configDir, "quota.txt")}, nil
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
	proxies := make(map[string]*chained.ChainedServerInfo)
	_, _, err := cf.openConfig(proxiesYaml, proxies, []byte{})
	if err != nil {
		return nil, err
	}

	dialers := make([]balancer.Dialer, 0, len(proxies))
	for name, s := range proxies {
		if s.PluggableTransport == "obfs4-tcp" {
			log.Debugf("Ignoring obfs4-tcp server: %v", name)
			// Ignore obfs4-tcp as these are already included as plain obfs4
			continue
		}
		dialer, err := c.chainedDialer(name, s)
		if err != nil {
			log.Errorf("Unable to configure chained server %v. Received error: %v", name, err)
			continue
		}
		log.Debugf("Adding chained server: %v", dialer.JustifiedLabel())
		dialers = append(dialers, dialer)
	}

	chained.TrackStatsFor(dialers)

	return dialers, nil
}

// chainedDialer creates a *balancer.Dialer backed by a chained server.
func (c *client) chainedDialer(name string, si *chained.ChainedServerInfo) (balancer.Dialer, error) {
	// Copy server info to allow modifying
	sic := &chained.ChainedServerInfo{}
	*sic = *si
	// Backwards-compatibility for clients that still have old obfs4
	// configurations on disk.
	if sic.PluggableTransport == "obfs4-tcp" {
		sic.PluggableTransport = "obfs4"
	}

	return chained.CreateDialer(name, sic, c.uc)
}

func trackAndLimitMemory() {
	for {
		time.Sleep(5 * time.Second)
		memstats := &runtime.MemStats{}
		runtime.ReadMemStats(memstats)
		log.Debugf("Memory InUse: %v    Alloc: %v    Sys: %v",
			humanize.Bytes(memstats.HeapInuse),
			humanize.Bytes(memstats.Alloc),
			humanize.Bytes(memstats.Sys))
		runtime.GC()
		debug.FreeOSMemory()
	}
}

func userConfigFor(deviceID string) *common.UserConfigData {
	return common.NewUserConfigData(
		deviceID,
		0,   // UserID currently unused
		"",  // Token currently unused
		nil, // Headers currently unused
		"",  // Language currently unused
	)
}
