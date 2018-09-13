// +build !disableresourcerandomization

package desktop

import (
	"crypto/rand"
	"encoding/hex"

	log "github.com/sirupsen/logrus"
)

const (
	defaultHTTPProxyAddress  = "127.0.0.1:0"
	defaultSOCKSProxyAddress = "127.0.0.1:0"
)

func randRead(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return hex.EncodeToString(buf)
}

// localHTTPToken fetches the local HTTP token from disk if it's there, and
// otherwise creates a new one and stores it.
func localHTTPToken(set *Settings) string {
	tok := set.GetLocalHTTPToken()
	if tok == "" {
		t := randRead(16)
		set.SetLocalHTTPToken(t)
		return t
	}
	return tok
}
