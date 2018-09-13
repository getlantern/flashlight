package autoupdate

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/autoupdate"
	"github.com/getlantern/i18n"
	"github.com/getlantern/notifier"
	log "github.com/sirupsen/logrus"

	"github.com/getlantern/flashlight/notifier"
	"github.com/getlantern/flashlight/proxied"
)

var (
	updateServerURL = "https://update.getlantern.org"
	PublicKey       = []byte(autoupdate.PackagePublicKey)
	Version         string

	cfgMutex           sync.RWMutex
	watchForUpdateOnce sync.Once
	httpClient         atomic.Value
	fnIconURL          func() string
)

// Configure sets the CA certificate to pin for the TLS auto-update connection.
func Configure(updateURL, updateCA string, iconURL func() string) {
	setUpdateURL(updateURL)
	fnIconURL = iconURL
	httpClient.Store(
		&http.Client{
			Transport: proxied.ChainedThenFrontedWith(updateCA),
		})
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
	watchForUpdateOnce.Do(func() {
		go watchForUpdate()
	})
}

func watchForUpdate() {
	log.Debugf("Software version: %s", Version)
	for {
		newVersion, err := autoupdate.ApplyNext(&autoupdate.Config{
			CurrentVersion: Version,
			CheckInterval:  4 * time.Hour,
			URL:            getUpdateURL(),
			PublicKey:      PublicKey,
			HTTPClient:     httpClient.Load().(*http.Client),
		})
		if err == nil {
			notifyUser(newVersion)
			log.Debugf("Got update for version %s", newVersion)
		} else {
			// unrecoverable error which tends to happen again
			log.Error(err)
		}
		// At this point we either updated the binary or failed to recover from a
		// update error, let's wait a bit longer before looking for another update.
		time.Sleep(24 * time.Hour)
	}
}

func notifyUser(newVersion string) {
	note := &notify.Notification{
		Title:      fmt.Sprintf(i18n.T("BACKEND_AUTOUPDATED_TITLE"), newVersion),
		Message:    fmt.Sprintf(i18n.T("BACKEND_AUTOUPDATED_MESSAGE"), newVersion),
		IconURL:    fnIconURL(),
		ClickLabel: i18n.T("BACKEND_CLICK_LABEL_GOT_IT"),
	}
	if !notifier.ShowNotification(note, "autoupdate-notification") {
		log.Debug("Unable to show autoupdate notification")
	}
}
