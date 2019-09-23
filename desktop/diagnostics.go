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

// When a user chooses to run diagnostics, we also attach a packet capture file, generated from the
// application's traffic log. This figure controls how far back in the log we go.
const captureSaveDuration = 5 * time.Minute

// runDiagnostics for the desktop app. Any of the return values may be nil.
func runDiagnostics(proxiesMap map[string]*chained.ChainedServerInfo, trafficLog *diagnostics.TrafficLog) (
	reportYAML []byte, zippedCapture []byte, errs []error) {

	var err error
	errs = []error{}

	reportYAML, err = yaml.Marshal(diagnostics.Run(proxiesMap))
	if err != nil {
		errs = append(errs, errors.New("failed to encode diagnostics report: %v", err))
	}

	proxyAddresses := []string{}
	for _, serverInfo := range proxiesMap {
		proxyAddresses = append(proxyAddresses, serverInfo.Addr)
	}
	zippedCapture, err = saveAndZipProxyTraffic(proxyAddresses, trafficLog)
	if err != nil {
		errs = append(errs, errors.New("failed to capture proxy traffic: %v", err))
	}

	if len(errs) == 0 {
		errs = nil
	}
	return
}

func saveAndZipProxyTraffic(addresses []string, trafficLog *diagnostics.TrafficLog) ([]byte, error) {
	zippedCapture := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zippedCapture)
	defer zipWriter.Close()

	captureWriter, err := zipWriter.Create("proxy-traffic.pcapng")
	if err != nil {
		return nil, errors.New("failed to create zip file for capture: %v", err)
	}
	for _, addr := range addresses {
		trafficLog.SaveCaptures(addr, captureSaveDuration)
	}
	if err := trafficLog.WritePcapng(captureWriter); err != nil {
		return nil, errors.New("failed to write saved captures to zip: %v", err)
	}
	return zippedCapture.Bytes(), nil
}
