package ui

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/getlantern/tarfs"

	"github.com/getlantern/flashlight/pro"
)

var (
	log = golog.LoggerFor("flashlight.ui")

	fs           *tarfs.FileSystem
	translations = eventual.NewValue()

	serve *server
)

func init() {
	// http.FileServer relies on OS to guess mime type, which can be wrong.
	// Override system default for current process.
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".js", "application/javascript")
}

// Start starts serving the UI.
func Start(requestedAddr, extURL, localHTTPTok, uiDomain string) error {
	serve = newServer(extURL, localHTTPTok, uiDomain)
	attachHandlers(serve)
	if err := serve.start(requestedAddr); err != nil {
		return err
	}
	return nil
}

func attachHandlers(s *server) {
	// This allows a second Lantern running on the system to trigger the existing
	// Lantern to show the UI, or at least try to
	startupHandler := func(resp http.ResponseWriter, req *http.Request) {
		s.show("existing", "lantern")
		resp.WriteHeader(http.StatusOK)
	}

	s.Handle("/startup", http.HandlerFunc(startupHandler))
	s.Handle("/pro/", pro.APIHandler())
	unpackUI()
	s.Handle(s.requestPath+"/", strippingHandler(http.FileServer(fs)))

}

// strippingHandler removes the secure request path from the URL so that the
// static file server can properly serve it (it's effectively a virtual path).
func strippingHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.Replace(r.URL.Path, serve.requestPath, "", -1)
		h.ServeHTTP(w, r)
	})
}

func Handle(pattern string, handler http.Handler) {
	serve.Handle(serve.requestPath+pattern, strippingHandler(handler))
}

// Stop stops the UI listener and all services. To facilitate test.
func Stop() {
	serve.stop()
}

func unpackUI() {
	var err error
	fs, err = tarfs.NewWithListener(Resources, "", fixPath)
	if err != nil {
		// Panicking here because this shouldn't happen at runtime unless the
		// resources were incorrectly embedded.
		panic(fmt.Errorf("Unable to open tarfs filesystem: %v", err))
	}
	translations.Set(fs.SubDir("locale"))
}

// fixPath changes the path in certain files that use hard coded absolute paths
// to include the secure random path instead of the naked root path the UI will
// just reject.
func fixPath(name string, file []byte) []byte {
	if strings.HasSuffix(name, ".css") {
		cur := bytes.Replace(file, []byte("/img/"), []byte(serve.requestPath+"/img/"), -1)
		return bytes.Replace(cur, []byte("/font/"), []byte(serve.requestPath+"/font/"), -1)
	}
	if strings.HasSuffix(name, ".html") {
		// This is just the favicon as of this writing.
		return bytes.Replace(file, []byte("href=\"/img/"), []byte("href=\""+serve.requestPath+"/img/"), -1)
	}
	return file
}

// Translations returns the translations for a given locale file.
func Translations(filename string) ([]byte, error) {
	log.Tracef("Accessing translations %v", filename)
	tr, ok := translations.Get(30 * time.Second)
	if !ok || tr == nil {
		return nil, fmt.Errorf("Could not get traslation for file name: %v", filename)
	}
	return tr.(*tarfs.FileSystem).Get(filename)
}

// GetUIAddr returns the current UI address.
func GetUIAddr() string {
	return serve.getUIAddr()
}

// Show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem.
func Show(campaign, medium string) {
	serve.show(campaign, medium)
}

// AddToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func AddToken(in string) string {
	return serve.addToken(in)
}

// ServeFromLocalUI serves the request using the local UI server. It relays
// the request like this because the local UI server uses the Go http package
// for things like websockets whereas the standard Lantern proxy does not.
// Relaying locally gives us the best of both worlds.
func ServeFromLocalUI(req *http.Request) *http.Request {
	if req.Method == http.MethodConnect {
		req.URL.Host = serve.listenAddr
	}
	// It's not clear why CONNECT requests also need the host header set here,
	// but it doesn't work without it.
	req.Host = serve.listenAddr
	return req
}
