package diagnostics

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/chained"
)

const captureSeconds = 10

// ErrorsMap represents multiple errors. ErrorsMap implements the error interface.
type ErrorsMap map[string]error

func (em ErrorsMap) Error() string {
	keys := []string{}
	for k := range em {
		keys = append(keys, k)
	}
	return fmt.Sprintf("errors for %s", strings.Join(keys, ", "))
}

// CaptureProxyTraffic generates a pcap file for each proxy in the input map. These files are saved
// in outputDir and named using the keys in proxiesMap.
//
// If an error is returned, it will be of type ErrorsMap. The keys of the map will be the keys in
// proxiesMap. If no error occurred for a given proxy, it will have no entry in the returned map.
//
// Expects tshark to be installed, otherwise returns errors.
func CaptureProxyTraffic(proxiesMap map[string]*chained.ChainedServerInfo, outputDir string) error {
	type captureError struct {
		proxyName string
		err       error
	}

	wg := new(sync.WaitGroup)
	captureErrors := make(chan captureError, len(proxiesMap))
	for proxyName, proxyInfo := range proxiesMap {
		wg.Add(1)
		go func(pName, pAddr string) {
			defer wg.Done()

			pHost, _, _ := net.SplitHostPort(pAddr)
			cmd := exec.Command(
				"tshark",
				"-w", filepath.Join(outputDir, fmt.Sprintf("%s.pcap", pName)),
				"-f", fmt.Sprintf("host %s", pHost),
				"-a", fmt.Sprintf("duration:%d", captureSeconds),
			)
			stdErr := new(bytes.Buffer)
			cmd.Stderr = stdErr
			if err := cmd.Run(); err != nil {
				captureErrors <- captureError{pName, errors.New("%v: %v", err, stdErr)}
			}
		}(proxyName, proxyInfo.Addr)
	}
	wg.Wait()
	close(captureErrors)

	errorsMap := ErrorsMap{}
	for capErr := range captureErrors {
		errorsMap[capErr.proxyName] = capErr.err
	}
	if len(errorsMap) == 0 {
		return nil
	}
	return &errorsMap
}
