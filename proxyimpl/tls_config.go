package proxyimpl

import (
	"io"
	"os"
	"sync"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/common"
)

var (
	tlsKeyLogWriter                 io.Writer
	createKeyLogWriterOnce          sync.Once
	InsecureSkipVerifyTLSMasqOrigin = false
)

func orderedCipherSuitesFromConfig(pc *config.ProxyConfig) []uint16 {
	if common.Platform == "android" {
		return mobileOrderedCipherSuites(pc)
	}
	return desktopOrderedCipherSuites(pc)
}

// Write the session keys to file if SSLKEYLOGFILE is set, same as browsers.
func getTLSKeyLogWriter() io.Writer {
	createKeyLogWriterOnce.Do(func() {
		path := os.Getenv("SSLKEYLOGFILE")
		if path == "" {
			return
		}
		var err error
		tlsKeyLogWriter, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			log.Debugf("Error creating keylog file at %v: %s", path, err)
		}
	})
	return tlsKeyLogWriter
}
