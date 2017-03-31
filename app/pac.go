package app

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/detour"
	"github.com/getlantern/filepersist"
	"github.com/getlantern/pac"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/icons"
	"github.com/getlantern/flashlight/ui"
	"github.com/getlantern/flashlight/util"
)

var (
	isPacOn     = int32(0)
	pacURL      string
	directHosts = make(map[string]bool)
	cfgMutex    sync.RWMutex
)

func servePACFile() {
	cfgMutex.Lock()
	defer cfgMutex.Unlock()
	if pacURL == "" {
		ui.Handle("/proxy_on.pac", http.HandlerFunc(pacFileHandler))
		// Trying to bypass Windows' PAC file cache.
		// This is a workaround for Windows 10 and Edge.
		//
		// Lantern changes the system's proxy settings a sets an URL like:
		//
		//   http://127.0.0.1:16823/path_to.pac?token=xxx
		//
		// This URL is verified by Windows, and if it works then the system sets it
		// as system proxy.
		//
		// The problem here was that, after rebooting, this URL was checked before
		// Lantern started, so it failed and was marked as invalid by the OS.
		//
		// After Lantern finally started and called pacOn() the URL was not being
		// verified again, because it was the same URL the system tried to reach a
		// few seconds before.
		//
		// Some browsers like Chrome or Firefox use the URL later in the game
		// anyway when Lantern is running, but some others like Edge do not even
		// try.
		//
		// By changing the URL here we are forcing the OS to check the URL whenever
		// Lantern starts.
		pacURL = addNoCache(ui.AddToken("/proxy_on.pac"))
	}
}

func pacFileHandler(resp http.ResponseWriter, req *http.Request) {
	log.Trace("Serving PAC file")
	resp.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	resp.WriteHeader(http.StatusOK)
	cfgMutex.RLock()
	defer cfgMutex.RUnlock()
	if _, err := genPACFile(resp); err != nil {
		log.Debugf("Error writing response: %v", err)
	}
}

func setUpPacTool() error {
	var iconFile string
	if runtime.GOOS == "darwin" {
		// We have to use a short filepath here because Cocoa won't display the
		// icon if the path is too long.
		iconFile = filepath.Join("/tmp", "escalatelantern.ico")
		icon, err := icons.Asset("icons/32on.ico")
		if err != nil {
			return fmt.Errorf("Unable to load escalation prompt icon: %v", err)
		}
		err = filepersist.Save(iconFile, icon, 0644)
		if err != nil {
			return fmt.Errorf("Unable to persist icon to disk: %v", err)
		}
		log.Debugf("Saved icon file to: %v", iconFile)
	}
	err := pac.EnsureHelperToolPresent("pac-cmd", "Lantern would like to be your system proxy", iconFile)
	if err != nil {
		return fmt.Errorf("Unable to set up pac setting tool: %v", err)
	}
	return nil
}

func genPACFile(w io.Writer) (int, error) {
	// TODO: we don't need to generate this thing everytime.

	hostsString := "[]"
	// only bypass sites if proxy all option is unset
	if !settings.GetProxyAll() {
		log.Trace("Not proxying all")
		var hosts []string
		for k, v := range directHosts {
			if v {
				hosts = append(hosts, k)
			}
		}
		hostsString = "['" + strings.Join(hosts, "', '") + "']"
	} else {
		log.Trace("Proxying all")
	}

	formatter :=
		`var bypassDomains = %s;
		function FindProxyForURL(url, host) {
			if (isPlainHostName(host) // including localhost
			|| shExpMatch(host, "*.local")) {
				return "DIRECT";
			}
			// only checks plain IP addresses to avoid leaking domain name
			if (/^[0-9.]+$/.test(host)) {
				if (isInNet(host, "10.0.0.0", "255.0.0.0") ||
				isInNet(host, "172.16.0.0",  "255.240.0.0") ||
				isInNet(host, "192.168.0.0",  "255.255.0.0") ||
				isInNet(host, "127.0.0.0", "255.255.255.0")) {
					return "DIRECT";
				}
			}
			// Lantern desktop version proxies only http(s) and ws(s)
			if (url.substring(0, 4) != 'http' && (url.substring(0, 2) != 'ws')) {
				return "DIRECT";
			}

			// If it's any plain text traffic, always proxy it.
			if (url.substring(0, 5) != 'https' && (url.substring(0, 2) != 'wss')) {
				return "PROXY %s; DIRECT";
			}
			for (var d in bypassDomains) {
				if (host == bypassDomains[d]) {
					return "DIRECT";
				}
			}
			return "PROXY %s; DIRECT";
		}`
	proxyAddr, ok := client.Addr(5 * time.Minute)
	if !ok {
		panic("Unable to get proxy address within 5 minutes")
	}
	proxyAddrString := proxyAddr.(string)
	log.Tracef("Setting proxy address to %v", proxyAddrString)
	return fmt.Fprintf(w, formatter, hostsString, proxyAddrString, proxyAddrString)
}

// watchDirectAddrs adds any site that has accessed directly without error to PAC file
func watchDirectAddrs() {
	go func() {
		for {
			addr := <-detour.DirectAddrCh
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				panic("watchDirectAddrs() got malformated host:port pair")
			}
			addDirectHost(host)
		}
	}()
}

func addDirectHost(host string) {
	cfgMutex.Lock()
	defer cfgMutex.Unlock()
	if !directHosts[host] {
		directHosts[host] = true
		cyclePAC()
	}
}

func pacOn() {
	log.Debugf("Setting lantern as system proxy via PAC URL at %v", pacURL)
	doPACOn(pacURL)
	atomic.StoreInt32(&isPacOn, 1)
}

func pacOff() {
	if atomic.CompareAndSwapInt32(&isPacOn, 1, 0) {
		log.Debug("Unsetting lantern as system proxy")
		doPACOff(pacURL)
	}
}

func cyclePAC() {
	log.Debug("Cycling the pac file")
	// prevents Lantern from accidently leave pac on after exits
	if atomic.LoadInt32(&isPacOn) == 1 {
		// reapply so browser will fetch the PAC URL again
		doPACOff(pacURL)
		doPACOn(pacURL)
	}
}

func doPACOn(pacURL string) {
	err := pac.On(pacURL)
	if err != nil {
		log.Errorf("Unable to set lantern as system proxy: %v", err)
	}
}

func doPACOff(pacURLorPrefix string) {
	err := pac.Off(pacURLorPrefix)
	if err != nil {
		log.Errorf("Unable to unset lantern as system proxy: %v", err)
		return
	}
	log.Debug("Unset lantern as system proxy")
}

func addNoCache(in string) string {
	return util.SetURLParam(in, "nocache", fmt.Sprintf("%v", time.Now().UnixNano()))
}
