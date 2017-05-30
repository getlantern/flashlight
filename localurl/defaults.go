// +build !disableresourcerandomization

package localurl

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("flashlight.localurl")

const defaultUIAddress = "localhost:0"

const strictOriginCheck = true

func randRead(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return hex.EncodeToString(buf)
}

// localHTTPToken returns the local HTTP token for accessing the proxy.
func localHTTPToken() string {
	return randRead(16)
}
