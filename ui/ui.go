package ui

import (
	"fmt"
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

	server *Server
)

// Start starts serving the UI.
func Start(requestedAddr string, allowRemote bool, extURL, localHTTPTok string) error {
	s := NewServer(requestedAddr, allowRemote, extURL, localHTTPTok)
	attachHandlers(s, allowRemote)

	if err := s.Start(); err != nil {
		return err
	}
	server = s
	return nil
}

func attachHandlers(s *Server, allowRemote bool) {
	// This allows a second Lantern running on the system to trigger the existing
	// Lantern to show the UI, or at least try to
	startupHandler := func(resp http.ResponseWriter, req *http.Request) {
		// If we're allowing remote, we're in practice not showing the UI on this
		// typically headless system, so don't allow triggering of the UI.
		if !allowRemote {
			s.Show()
		}
		resp.WriteHeader(http.StatusOK)
	}

	s.Handle("/pro/", pro.APIHandler())
	s.Handle("/startup", http.HandlerFunc(startupHandler))
	unpackUI()
	s.Handle("/", http.FileServer(fs))

}

func Handle(pattern string, handler http.Handler) {
	server.Handle(pattern, handler)
}

// Stop stops the UI listener and all services. To facilitate test.
func Stop() {
	unregisterAll()
	server.Stop()
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

// GetUIAddr returns the current UI address.
func GetUIAddr() string {
	if server == nil {
		return ""
	}
	return server.GetUIAddr()
}

func Show() {
	if server == nil {
		return
	}
	server.Show()
}

// AddToken adds the UI domain and custom request token to the specified
// request path. Without that token, the backend will reject the request to
// avoid web sites detecting Lantern.
func AddToken(in string) string {
	if server == nil {
		return ""
	}
	return server.AddToken(in)
}
