package flashlight

import (
	"fmt"
	"net"
	"sync"
	"time"

	commonconfig "github.com/getlantern/common/config"
	"github.com/getlantern/dnsgrab"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/netx"

	"github.com/getlantern/flashlight/v7/client"
	"github.com/getlantern/flashlight/v7/common"
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
	isPro      func() bool
	autoReport func() bool
	client     *client.Client
	clientMu   sync.Mutex
	op         *fops.Op
}

// Client returns the HTTP client proxy this instance of flashlight is currently configured with
func (f *Flashlight) Client() *client.Client {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()
	client := f.client
	return client
}

func (f *Flashlight) SetClient(client *client.Client) {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()
	f.client = client
}

// New creates a client proxy.
func New(
	appName string,
	appVersion string,
	revisionDate string,
	configDir string,
	enableVPN bool,
	disconnected func() bool,
	allowPrivateHosts func() bool,
	autoReport func() bool,
	flagsAsMap map[string]interface{},
	userConfig common.UserConfig,
	statsTracker stats.Tracker,
	isPro func() bool,
	lang func() string,
	reverseDNS func(host string) (string, error),
	eventWithLabel func(category, action, label string),
	options ...client.Option,
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
		f.isPro,
		lang,
		reverseDNS,
		eventWithLabel,
		options...,
	)
	if err != nil {
		fatalErr := fmt.Errorf("unable to initialize client: %v", err)
		f.op.FailIf(fatalErr)
		f.op.End()
		return nil, fatalErr
	}
	f.SetClient(cl)
	return f, nil
}

// Run starts background services and runs the client proxy. It blocks as long as
// the proxy is running.
func (f *Flashlight) Run(httpProxyAddr, socksProxyAddr string,
	afterStart func(cl *client.Client),
	onError func(err error),
) {
	f.Client().Start(httpProxyAddr, socksProxyAddr, afterStart, onError)
}

// Stops the local proxy
func (f *Flashlight) Stop() error {
	return f.Client().Stop()
}

func displayVersion(appVersion, revisionDate string) {
	log.Debugf("---- application version: %s, library version: %s, build revision date: %s ----",
		appVersion, common.LibraryVersion, revisionDate)
}
