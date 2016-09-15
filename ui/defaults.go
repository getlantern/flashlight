// +build !disableresourcerandomization

package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const defaultUIAddress = "127.0.0.1:0"

const strictOriginCheck = true

func randRead(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return hex.EncodeToString(buf)
}

func proxyDomain() string {
	return fmt.Sprintf("%s.lantern.io", randRead(4))
}

func token() string {
	return randRead(16)
}
