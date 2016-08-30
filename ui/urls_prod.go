// +build prod

package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const defaultUIAddress = "127.0.0.1:0"

// pacPath returns a random path for the PAC file.
func pacPath() string {
	randURL := make([]byte, 16)
	if _, err := rand.Read(randURL); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return fmt.Sprintf("/%s/proxy_on.pac", hex.EncodeToString(randURL))
}

func proxyDomain() string {
	randName := make([]byte, 4)
	if _, err := rand.Read(randName); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return fmt.Sprintf("%s.lantern.io", hex.EncodeToString(randName))
}
