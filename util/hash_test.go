package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
)

func TestGetFileHash(t *testing.T) {
	wd, _ := os.Getwd()
	path := wd + "/hash.go"

	hash, _ := GetFileHash(path)
	//log.Infof("Got hash! %x", hash)
	log.Infof("Got hash! %v", hash)

	// Update this with shasum -a 256 hash.go
	assert.Equal(t, "a7db9a14837bdb7192b24940b8ef3bc4a127cf002248bacdf9e7165ecd41d04e", hash,
		"hashes not equal! has hashes.go changed?")
}
