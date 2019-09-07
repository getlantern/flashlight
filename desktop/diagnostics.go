package desktop

import (
	"archive/zip"
	"bytes"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/diagnostics"
	"github.com/getlantern/yaml"
)

// When a user chooses to run diagnostics, we will capture all proxy traffic for captureDuration.
const captureDuration = 30 * time.Second

// runDiagnostics for the desktop app. Any of the return values may be nil.
func runDiagnostics(proxiesMap map[string]*chained.ChainedServerInfo) (
	reportYAML []byte, zippedCapture []byte, errs []error) {

	var err error
	errs = []error{}

	reportYAML, err = yaml.Marshal(diagnostics.Run(proxiesMap))
	if err != nil {
		errs = append(errs, errors.New("failed to encode diagnostics report: %v", err))
	}
	zippedCapture, err = captureAndZipProxyTraffic(proxiesMap)
	if err != nil {
		errs = append(errs, errors.New("failed to capture proxy traffic: %v", err))
	}

	if len(errs) == 0 {
		errs = nil
	}
	return
}

func captureAndZipProxyTraffic(proxiesMap map[string]*chained.ChainedServerInfo) ([]byte, error) {
	zippedCapture := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zippedCapture)
	defer zipWriter.Close()

	captureWriter, err := zipWriter.Create("proxy-traffic.pcapng")
	if err != nil {
		return nil, errors.New("failed to create zip file for capture: %v", err)
	}

	captureConfig := diagnostics.CaptureConfig{
		StopChannel: diagnostics.CloseAfter(captureDuration),
		Output:      captureWriter,
	}
	if err = diagnostics.CaptureProxyTraffic(proxiesMap, &captureConfig); err != nil {
		return nil, err
	}
	return zippedCapture.Bytes(), nil
}
