package autoupdate

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/autoupdate"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/golog"
)

var (
	log             = golog.LoggerFor("flashlight.autoupdate")
	updateServerURL = "https://update.getlantern.org"
	PublicKey       = []byte(autoupdate.PackagePublicKey)
	Version         string

	cfgMutex    sync.RWMutex
	updateMutex sync.Mutex

	watching int32

	applyNextAttemptTime = time.Hour * 2

	httpClient = &http.Client{
		Transport: proxied.ChainedThenFrontedWith("d2yl1zps97e5mx.cloudfront.net"),
	}
)

// Configure sets the CA certificate to pin for the TLS auto-update connection.
func Configure(updateURL, updateCA string) {
	setUpdateURL(updateURL)
	enableAutoupdate()
}

func setUpdateURL(url string) {
	if url == "" {
		return
	}
	cfgMutex.Lock()
	defer cfgMutex.Unlock()
	updateServerURL = url
}

func getUpdateURL() string {
	cfgMutex.RLock()
	defer cfgMutex.RUnlock()
	return updateServerURL + "/update"
}

func enableAutoupdate() {
	go watchForUpdate()
}

func watchForUpdate() {
	if atomic.LoadInt32(&watching) < 1 {

		atomic.AddInt32(&watching, 1)

		log.Debugf("Software version: %s", Version)

		for {
			applyNext()
			// At this point we either updated the binary or failed to recover from a
			// update error, let's wait a bit before looking for a another update.
			time.Sleep(applyNextAttemptTime)
		}
	}
}

func applyNext() {
	updateMutex.Lock()
	defer updateMutex.Unlock()

	err := autoupdate.ApplyNext(&autoupdate.Config{
		CurrentVersion: Version,
		URL:            getUpdateURL(),
		PublicKey:      PublicKey,
		HTTPClient:     httpClient,
	})
	if err != nil {
		log.Debugf("Error getting update: %v", err)
		return
	}
	log.Debugf("Got update.")

}
