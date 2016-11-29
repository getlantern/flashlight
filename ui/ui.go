package ui

import (
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/edgedetect"
	"github.com/getlantern/eventual"
	"github.com/getlantern/flashlight/pro"
	"github.com/getlantern/golog"
	"github.com/getlantern/tarfs"
	"github.com/skratchdot/open-golang/open"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/flashlight/util"
)

var (
	log = golog.LoggerFor("flashlight.ui")

	l                  net.Listener
	fs                 *tarfs.FileSystem
	translations       = eventual.NewValue()
	server             *http.Server
	uiaddr             string
	allowRemoteClients bool
	proxiedUIAddr      string
	preferProxiedUI    int32
	edge               = edgedetect.DefaultBrowserIsEdge()

	openedExternal = false
	externalURL    string
	r              = NewServeMux()
	localHTTPToken string
)

func Handle(pattern string, handler http.Handler) {
	r.Handle(pattern, handler)
}

// Start starts serving the UI.
func Start(requestedAddr string, allowRemote bool, extURL, localHTTPTok string) error {
	localHTTPToken = localHTTPTok
	addr, err := normalizeAddr(requestedAddr)
	if err != nil {
		return fmt.Errorf("Unable to resolve UI address: %v", err)
	}
	if allowRemote {
		// If we want to allow remote connections, we have to bind all interfaces
		addr = &net.TCPAddr{Port: addr.Port}
	}

	l, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return fmt.Errorf("Unable to listen at %v. Error is: %v", addr, err)
	}

	// Setting port (case when port was 0)
	addr.Port = l.Addr().(*net.TCPAddr).Port

	// Updating listenAddr
	listenAddr := addr.String()

	unpackUI()

	externalURL = extURL

	allowRemoteClients = allowRemote

	// This allows a second Lantern running on the system to trigger the existing
	// Lantern to show the UI, or at least try to
	startupHandler := func(resp http.ResponseWriter, req *http.Request) {
		// If we're allowing remote, we're in practice not showing the UI on this
		// typically headless system, so don't allow triggering of the UI.
		if !allowRemote {
			Show()
		}
		resp.WriteHeader(http.StatusOK)
	}

	r.Handle("/pro/", pro.APIHandler())
	r.Handle("/startup", http.HandlerFunc(startupHandler))
	r.Handle("/", http.FileServer(fs))

	server = &http.Server{
		Handler:  r,
		ErrorLog: log.AsStdLogger(),
	}
	go func() {
		err := server.Serve(l)
		if err != nil {
			log.Errorf("Error serving: %v", err)
		}
	}()

	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		log.Fatalf("Could not parse host:port on %q", listenAddr)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	uiaddr = fmt.Sprintf("%s:%s", host, port)

	// Note - we display the UI using the LanternSpecialDomain. This is necessary
	// for Microsoft Edge on Windows 10 because, being a Windows Modern App, its
	// default network isolation settings prevent it from opening websites on the
	// loopback address. We get around this by exploiting the fact that Edge will
	// happily connect to our proxy server running on the loopback interface. So,
	// we use what looks like a real domain for the UI (ui.lantern.io), the proxy
	// detects this and reroutes the traffic to the local UI server. The proxy is
	// allowed to connect to loopback because it doesn't have the same restriction
	// as Microsoft Edge.
	proxiedUIAddr = proxyDomainFor(listenAddr)
	client.SetProxyUIAddr(proxiedUIAddr, listenAddr)

	log.Debugf("UI available at http://%v and http://%v", uiaddr, proxiedUIAddr)

	return nil
}

// Stop stops the UI listener and all services. To facilitate test.
func Stop() {
	unregisterAll()
	// Reset it here instead of changing how r is initialized, to avoid
	// bringing in unexpected bugs.
	r = NewServeMux()
	if l != nil {
		l.Close()
	}
}

func normalizeAddr(requestedAddr string) (*net.TCPAddr, error) {
	var addr string
	if requestedAddr == "" {
		addr = defaultUIAddress
	} else if strings.HasPrefix(requestedAddr, "http://") {
		log.Errorf("Client tried to start at bad address: %v", requestedAddr)
		addr = strings.TrimPrefix(requestedAddr, "http://")
	} else {
		addr = requestedAddr
	}

	return net.ResolveTCPAddr("tcp4", addr)
}

func unpackUI() {
	var err error
	fs, err = tarfs.New(Resources, "")
	if err != nil {
		// Panicking here because this shouldn't happen at runtime unless the
		// resources were incorrectly embedded.
		panic(fmt.Errorf("Unable to open tarfs filesystem: %v", err))
	}
	translations.Set(fs.SubDir("locale"))
}

// Translations returns the translations for a given locale file.
func Translations(filename string) ([]byte, error) {
	log.Debugf("Accessing translations")
	tr, ok := translations.Get(30 * time.Second)
	if !ok || tr == nil {
		return nil, fmt.Errorf("Could not get traslation for file name: %v", filename)
	}
	return tr.(*tarfs.FileSystem).Get(filename)
}

// PreferProxiedUI returns the preferred address to serve the UI based on
// whether or not the user's default browser is Edge. This is because Edge
// will not allow connections to localhost by default, but we can work
// around it with our speciial domain.
func PreferProxiedUI(val bool) (newAddr string, addrChanged bool) {
	previousPreferredUIAddr := GetPreferredUIAddr()
	updated := int32(0)
	if val {
		updated = 1
	}
	atomic.StoreInt32(&preferProxiedUI, updated)
	newPreferredUIAddr := GetPreferredUIAddr()
	return newPreferredUIAddr, newPreferredUIAddr != previousPreferredUIAddr
}

func shouldPreferProxiedUI() bool {
	return atomic.LoadInt32(&preferProxiedUI) == 1
}

// GetPreferredUIAddr returns the address to access the UI on, which depends
// on whether or not we're running on Edge essgentially.
func GetPreferredUIAddr() string {
	// We only use the proxied UI address if the default browser is Microsoft Edge
	if edge && shouldPreferProxiedUI() {
		log.Debugf("Returning '%v' for proxied UI addr", proxiedUIAddr)
		return proxiedUIAddr
	}
	log.Debugf("Returning plain uiaddr %v", uiaddr)
	return uiaddr
}

// GetDirectUIAddr returns the current UI address when accessing directly.
func GetDirectUIAddr() string {
	return uiaddr
}

// Show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func Show() {
	go func() {
		uiURL := fmt.Sprintf("http://%s/?1", GetPreferredUIAddr())
		log.Debugf("Opening browser at %v", uiURL)
		err := open.Run(uiURL)
		if err != nil {
			log.Errorf("Error opening page to `%v`: %v", uiURL, err)
		}

		// This is for opening exernal URLs in a new browser window for partners
		// such as Manoto.
		onceBody := func() {
			openExternalURL(externalURL)
		}
		var run sync.Once
		run.Do(onceBody)
	}()
}

// openExternalUrl opens an external URL of one of our partners automatically
// at startup if configured to do so. It should only open the first time in
// a given session that Lantern is opened.
func openExternalURL(u string) {
	var url string
	if u == "" {
		return
	} else if strings.HasPrefix(u, "https://www.manoto1.com/") || strings.HasPrefix(u, "https://www.facebook.com/manototv") {
		// Here we make sure to override any old manoto URLs with the latest.
		url = "https://www.manototv.com/iran?utm_campaign=manotolantern"
	} else {
		url = u
	}
	time.Sleep(4 * time.Second)
	err := open.Run(url)
	if err != nil {
		log.Errorf("Error opening external page to `%v`: %v", uiaddr, err)
	}
}

// AddToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func AddToken(in string) string {
	return util.SetURLParam("http://"+path.Join(uiaddr, in), "token", localHTTPToken)
}
