package desktop

import (
	"bytes"
	"compress/gzip"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/diagnostics"
	"github.com/getlantern/trafficlog-flashlight/tlproc"
	"github.com/getlantern/yaml"
)

// When a user chooses to run diagnostics, we also attach a packet capture file, generated from the
// application's traffic log. This figure controls how far back in the log we go.
const captureSaveDuration = 5 * time.Minute

// collectDiagnostics for the desktop app. trafficLog may be nil. Any of the return values may be nil.
func collectDiagnostics(proxies []balancer.Dialer, trafficLog *tlproc.TrafficLogProcess) (
	reportYAML []byte, gzippedPcapng []byte, errs []error) {

	var err error
	errs = []error{}

	reportYAML, err = yaml.Marshal(diagnostics.Run(proxies))
	if err != nil {
		errs = append(errs, errors.New("failed to encode diagnostics report: %v", err))
	}

	if trafficLog != nil {
		proxyAddresses := []string{}
		for _, p := range proxies {
			proxyAddresses = append(proxyAddresses, p.Addr())
		}
		gzippedPcapng, err = saveAndZipProxyTraffic(proxyAddresses, trafficLog)
		if err != nil {
			errs = append(errs, errors.New("failed to capture proxy traffic: %v", err))
		}
	}

	if len(errs) == 0 {
		errs = nil
	}
	return
}

// Saves proxy traffic for captureSaveDuration and gzips the resulting pcapng.
func saveAndZipProxyTraffic(addresses []string, trafficLog *tlproc.TrafficLogProcess) ([]byte, error) {
	buf := new(bytes.Buffer)
	gzipW := gzip.NewWriter(buf)
	for _, addr := range addresses {
		if err := trafficLog.SaveCaptures(addr, captureSaveDuration); err != nil {
			return nil, errors.New("failed to save captures: %v", err)
		}
	}
	if err := trafficLog.WritePcapng(gzipW); err != nil {
		return nil, errors.New("failed to write saved captures to zip: %v", err)
	}
	if err := gzipW.Close(); err != nil {
		return nil, errors.New("failed to close gzip writer: %v", err)
	}
	return buf.Bytes(), nil
}
