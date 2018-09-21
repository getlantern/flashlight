package util

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/getlantern/zaplog"
)

// GetFileHash returns the hex encoding of the sha-256 hash of the
// file at the specified path.
func GetFileHash(path string) (string, error) {
	log := logging.LoggerFor("flashlight.util")
	log.Infof("Hashing file at path %v", path)
	if f, err := os.Open(path); err != nil {
		log.Errorf("Could not open file at %v, %v", path, err)
		return "", err
	} else {
		defer f.Close()
		hasher := sha256.New()
		if _, e := io.Copy(hasher, f); e != nil {
			log.Error(e)
			return "", e
		} else {
			sum := hasher.Sum(nil)
			return hex.EncodeToString(sum), nil
		}
	}
}
