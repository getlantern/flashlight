package desktop

import (
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/diagnostics"
	"github.com/getlantern/yaml"
)

func runDiagnostics(proxiesMap map[string]*chained.ChainedServerInfo) (reportYAML []byte, err error) {
	r := diagnostics.Run(proxiesMap)
	b, err := yaml.Marshal(r)
	if err != nil {
		log.Debugf("the following report failed to encode: %+v", r)
		return nil, errors.New("failed to encode diagnostics report: %v", err)
	}
	return b, nil
}
