package config

import (
	"compress/gzip"
	"io/ioutil"
	"os"
	"testing"

	"github.com/getlantern/rot13"
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

func writeObfuscatedConfig(t *testing.T, filename string, obfuscatedFilename string) {
	log.Debugf("Writing obfuscated config from %v to %v", filename, obfuscatedFilename)

	// open and read yaml file
	yamlFile, err := os.Open(filename)
	if err != nil {
		t.Fatalf("Failed to open %v: %v", filename, err)
	}
	defer yamlFile.Close()

	bytes, err := ioutil.ReadAll(yamlFile)
	if err != nil {
		t.Fatalf("Failed to read %v: %v", filename, err)
	}

	// create new obfuscated file
	outfile, err := os.OpenFile(obfuscatedFilename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Unable to open file %v for writing: %v", obfuscatedFilename, err)
	}
	defer outfile.Close()

	// write ROT13-encoded config to obfuscated file
	out := rot13.NewWriter(outfile)
	_, err = out.Write(bytes)
	if err != nil {
		t.Fatalf("Unable to write yaml to file %v: %v", obfuscatedFilename, err)
	}
}

// Certain tests fetch global config from a remote server and store it at
// `global.yaml`.  Other tests rely on `global.yaml` matching the
// `fetched-global.yaml` fixture.  For tests that fetch config remotely, we must
// delete the config file once the test has completed to avoid causing other
// these other tests to fail in the event that the remote config differs from
// the fixture.
func deleteGlobalConfig() {
	os.Remove("global.yaml")
}
