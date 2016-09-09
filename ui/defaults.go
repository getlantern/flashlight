// +build !disableresourcerandomization

package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const defaultUIAddress = "127.0.0.1:0"

func randRead(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return hex.EncodeToString(buf)
}

// pacPath returns a random path for the PAC file.
func pacPath() string {
	return fmt.Sprintf("/%s/proxy_on.pac", randRead(16))
}

func proxyDomain() string {
	return fmt.Sprintf("%s.lantern.io", randRead(4))
}
