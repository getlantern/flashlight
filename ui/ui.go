package ui

import (
	"fmt"
	"mime"
	"net/http"
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
func Start(requestedAddr, extURL, localHTTPTok string) error {
	serve = newServer(extURL, localHTTPTok)
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
		s.showRoot("existing", "lantern")
		resp.WriteHeader(http.StatusOK)
	}
	s.Handle("/startup", http.HandlerFunc(startupHandler))
	s.Handle("/pro/", pro.APIHandler())
	unpackUI()
	s.Handle("/", http.FileServer(fs))

}

func Handle(pattern string, handler http.Handler) {
	serve.Handle(pattern, handler)
}

// Stop stops the UI listener and all services. To facilitate test.
func Stop() {
	serve.stop()
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

// ShowRoot is like Show using the default (root) URL for the UI.
func ShowRoot(campaign, medium string) {
	serve.showRoot(campaign, medium)
}

// Show opens the UI in a browser. Note we know the UI server is
// *listening* at this point as long as Start is correctly called prior
// to this method. It may not be reading yet, but since we're the only
// ones reading from those incoming sockets the fact that reading starts
// asynchronously is not a problem. destURL indicates which URL to open.
func Show(destURL, campaign, medium string) {
	serve.show(destURL, campaign, medium)
}

// AddToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func AddToken(in string) string {
	return serve.addToken(in)
}
