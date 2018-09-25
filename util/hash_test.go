package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFileHash(t *testing.T) {
	wd, _ := os.Getwd()
	path := wd + "/hash.go"

	hash, _ := GetFileHash(path)

	// Update this with shasum -a 256 hash.go
	assert.Equal(t, "ee61be633b6e2416e95a54b5b2b9effb2870520abc8911303e4b0649b7c1bdde", hash,
		"hashes not equal! has hashes.go changed?")
}
