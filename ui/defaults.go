// +build !disableresourcerandomization

package ui

import (
	"crypto/rand"
	"encoding/hex"
)

var defaultUIAddresses = []string{"localhost:0", "127.0.0.1:0"}

const strictOriginCheck = true

func randRead(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return hex.EncodeToString(buf)
}

// LocalHTTPToken returns the local HTTP token for accessing the proxy.
func LocalHTTPToken() string {
	return randRead(16)
}
