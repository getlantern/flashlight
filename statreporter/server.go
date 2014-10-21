package statreporter

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

var (
	SERVER_INTERVAL = 20 * time.Second
)

type ServerReporter struct {
	Reporter
	bytesGiven int64 // tracks bytes given
}

// OnBytesGiven registers the fact that bytes were given (sent or received)
func (reporter *ServerReporter) OnBytesGiven(clientIp string, bytes int64) {
	atomic.AddInt64(&reporter.bytesGiven, bytes)
}

// reportStats periodically reports the stats to statshub via HTTP post
func (reporter *ServerReporter) Start() {
	for {
		nextInterval := time.Now().Truncate(SERVER_INTERVAL).Add(SERVER_INTERVAL)
		waitTime := nextInterval.Sub(time.Now())
		time.Sleep(waitTime)
		bytesGiven := atomic.SwapInt64(&reporter.bytesGiven, 0)
		err := reporter.postGiveStats(bytesGiven)
		if err != nil {
			log.Errorf("Error on posting stats: %s", err)
		} else {
			log.Debugf("Reported %d bytesGiven to statshub", bytesGiven)
		}
	}
}

func (reporter *ServerReporter) postGiveStats(bytesGiven int64) error {
	report := map[string]interface{}{
		"dims": map[string]string{
			"country": reporter.Country,
		},
		"increments": map[string]int64{
			"bytesGiven":             bytesGiven,
			"bytesGivenByFlashlight": bytesGiven,
		},
	}

	jsonBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("Unable to marshal json for stats: %s", err)
	}

	return reporter.postStats(jsonBytes)
}
