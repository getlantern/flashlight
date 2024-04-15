package flashlight

import (
	"fmt"
	"net"
	"time"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/netx"

	"github.com/getlantern/flashlight/v7/bandit"
	"github.com/getlantern/flashlight/v7/client"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/email"
	"github.com/getlantern/flashlight/v7/geolookup"
	fops "github.com/getlantern/flashlight/v7/ops"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/getlantern/flashlight/v7/shortcut"
	"github.com/getlantern/flashlight/v7/stats"
)

var (
	log = golog.LoggerFor("flashlight")
)

func init() {
	netx.EnableNAT64AutoDiscovery()
}

type ProxyListener interface {
	OnProxies(map[string]*commonconfig.ProxyConfig)
}

type Flashlight struct {
	configDir  string
	flagsAsMap map[string]interface{}
	userConfig common.UserConfig
	isPro      func() bool
	autoReport func() bool
	client     *client.Client
	op         *fops.Op
}

// EnableNamedDomainRules adds named domain rules specified as arguments to the domainrouting rules table
func (f *Flashlight) EnableNamedDomainRules(names ...string) {
	f.client.EnableNamedDomainRules(names...)
}

// DisableNamedDomainRules removes named domain rules specified as arguments from the domainrouting rules table
func (f *Flashlight) DisableNamedDomainRules(names ...string) {
	f.DisableNamedDomainRules(names...)
}

// New creates a client proxy.
func New(
	appName string,
	appVersion string,
	revisionDate string,
	configDir string,
	enableVPN bool,
	disconnected func() bool,
	_proxyAll func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	onConfigUpdate func(*config.Global, config.Source),
	onReady func(bool),
	onProxiesUpdate func([]bandit.Dialer, config.Source),
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	isPro func() bool,
	lang func() string,
	reverseDNS func(host string) (string, error),
	eventWithLabel func(category, action, label string),
) (*Flashlight, error) {
	log.Debugf("Running in app: %v", appName)
	log.Debugf("Using configdir: %v", configDir)

	displayVersion(appVersion, revisionDate)
	common.CompileTimeApplicationVersion = appVersion
	deviceID := userConfig.GetDeviceID()
	log.Debugf("You can query for this device's activity under device id: %v", deviceID)
	fops.InitGlobalContext(appName, appVersion, revisionDate, deviceID, isPro, func() string { return geolookup.GetCountry(0) })
	email.SetHTTPClient(proxied.DirectThenFrontedClient(1 * time.Minute))

	f := &Flashlight{
		configDir:  configDir,
		flagsAsMap: flagsAsMap,
		userConfig: userConfig,
		isPro:      isPro,
		autoReport: autoReport,
		op:         fops.Begin("client_started"),
	}

	var grabber dnsgrab.Server
	var grabberErr error
	if enableVPN {
		grabber, grabberErr = dnsgrab.Listen(50000,
			"127.0.0.1:53",
			func() string { return "8.8.8.8" })
		if grabberErr != nil {
			log.Errorf("dnsgrab unable to listen: %v", grabberErr)
		}

		go func() {
			if err := grabber.Serve(); err != nil {
				log.Errorf("dnsgrab stopped serving: %v", err)
			}
		}()

		reverseDNS = func(addr string) (string, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				host = addr
			}
			ip := net.ParseIP(host)
			if ip == nil {
				log.Debugf("Unable to parse IP %v, passing through address as is", host)
				return addr, nil
			}
			updatedHost, ok := grabber.ReverseLookup(ip)
			if !ok {
				// This means that the IP is one of our fake IPs (like 240.0.0.5) but dnsgrab doesn't know it. We cache dnsgrab entries
				// on disk for 24 hours, so this should almost never happen.
				return "", errors.New("Invalid IP address")
			}
			if splitErr != nil {
				return updatedHost, nil
			}
			return fmt.Sprintf("%v:%v", updatedHost, port), nil
		}
	}

	cl, err := client.NewClient(
		f.configDir,
		f.op,
		disconnected,
		flagsAsMap,
		shortcut.Allow,
		userConfig,
		statsTracker,
		allowPrivateHosts,
		lang,
		reverseDNS,
		eventWithLabel,
		client.WithConfig(onConfigUpdate),
		client.WithReady(onReady),
		client.WithIsPro(isPro),
		client.WithProxies(onProxiesUpdate),
	)
	if err != nil {
		fatalErr := fmt.Errorf("unable to initialize client: %v", err)
		f.op.FailIf(fatalErr)
		f.op.End()
		return nil, fatalErr
	}

	f.client = cl
	return f, nil
}

// Run starts background services and runs the client proxy. It blocks as long as
// the proxy is running.
func (f *Flashlight) Run(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
	f.client.Start(httpProxyAddr, socksProxyAddr, afterStart, onError)
}

// Stops the local proxy
func (f *Flashlight) Stop() error {
	return f.client.Stop()
}

func displayVersion(appVersion, revisionDate string) {
	log.Debugf("---- application version: %s, library version: %s, build revision date: %s ----",
		appVersion, common.LibraryVersion, revisionDate)
}
