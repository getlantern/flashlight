package util

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
)

// GetFileHash returns the hex encoding of the sha-256 hash of the
// file at the specified path.
func GetFileHash(path string) (string, error) {
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
