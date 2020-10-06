package desktop

import (
	"bytes"
	"compress/gzip"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/diagnostics"
	"github.com/getlantern/yaml"
)

func (app *App) runDiagnostics() (reportYAML, gzippedPcapng []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("recovered from panic while collecting diagnostics: %v", r)
		}
	}()

	errs := []error{}
	reportYAML, err = yaml.Marshal(diagnostics.Run(app.flashlight.GetProxies()))
	if err != nil {
		errs = append(errs, err)
	}
	gzippedPcapng, err = app.saveAndZipProxyTraffic()
	if err != nil {
		errs = append(errs, err)
	}
	return reportYAML, gzippedPcapng, combineErrors(errs...)
}

// Saves proxy traffic for captureSaveDuration and gzips the resulting pcapng.
func (app *App) saveAndZipProxyTraffic() ([]byte, error) {
	buf := new(bytes.Buffer)
	gzipW := gzip.NewWriter(buf)
	if err := app.flashlight.GetCapturedPackets(gzipW); err != nil {
		return nil, err
	}
	if err := gzipW.Close(); err != nil {
		return nil, errors.New("failed to close gzip writer: %v", err)
	}
	return buf.Bytes(), nil
}

func combineErrors(errs ...error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errors.New("multiple errors: %v", errs)
	}
}
