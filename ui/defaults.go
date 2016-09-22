// +build !disableresourcerandomization

package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/crc32"
)

const defaultUIAddress = "127.0.0.1:0"

const strictOriginCheck = true

func proxyDomainFor(addr string) string {
	cksm := crc32.Checksum([]byte(addr), crc32.MakeTable(crc32.IEEE))
	return fmt.Sprintf("%x.lantern.io", cksm)
}

func randRead(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("Failed to get random bytes: %s", err)
	}
	return hex.EncodeToString(buf)
}

func token() string {
	return randRead(16)
}
