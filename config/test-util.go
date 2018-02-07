package config

import (
	"compress/gzip"
	"io/ioutil"
	"os"
	"testing"
)

func createGzippedFile(t *testing.T, filename string, gzippedFilename string) {
	dat, _ := ioutil.ReadFile(filename)

	gzippedFile, err := os.Create(gzippedFilename)
	if err != nil {
		t.Fatalf("Could not create file %v: %v", gzippedFile, err)
	}

	w := gzip.NewWriter(gzippedFile)
	w.Name = gzippedFilename

	_, err = w.Write(dat)
	if err != nil {
		t.Fatalf("Could not write %v: %v", gzippedFilename, err)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("Could not close %v: %v", gzippedFilename, err)
	}
}
