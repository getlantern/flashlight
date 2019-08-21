package desktop

import (
	"bytes"
	"io/ioutil"
	"path/filepath"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/diagnostics"
	"github.com/getlantern/flashlight/util"
	"github.com/getlantern/yaml"
)

// runDiagnostics for the desktop app. Any of the return values may be nil.
func runDiagnostics(proxiesMap map[string]*chained.ChainedServerInfo) (
	reportYAML []byte, zippedPcapFiles []byte, errs []error) {

	var err error
	errs = []error{}

	reportYAML, err = yaml.Marshal(diagnostics.Run(proxiesMap))
	if err != nil {
		errs = append(errs, errors.New("failed to encode diagnostics report: %v", err))
	}
	zippedPcapFiles, err = captureAndZipProxyTraffic(proxiesMap)
	if err != nil {
		errs = append(errs, errors.New("failed to capture proxy traffic: %v", err))
	}

	if len(errs) == 0 {
		errs = nil
	}
	return
}

func captureAndZipProxyTraffic(proxiesMap map[string]*chained.ChainedServerInfo) ([]byte, error) {
	tmpDir, err := ioutil.TempDir("", "lantern-diagnostics-captures")
	if err != nil {
		return nil, errors.New("failed to create temporary directory for pcap files: %v", err)
	}
	if err := diagnostics.CaptureProxyTraffic(proxiesMap, tmpDir); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	opts := util.ZipOptions{Globs: map[string]string{"captures-by-proxy": filepath.Join(tmpDir, "*")}}
	if err := util.ZipFiles(buf, opts); err != nil {
		return nil, errors.New("failed to zip pcap files: %v", err)
	}
	return buf.Bytes(), nil
}
