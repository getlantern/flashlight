package statreporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/getlantern/flashlight/log"
)

const (
	STATSHUB_URL_TEMPLATE = "https://pure-journey-3547.herokuapp.com/stats/%s"
	REPORT_STATS_INTERVAL = 20 * time.Second
)

type Reporter struct {
	InstanceId string // (optional) instanceid under which to report statistics
	Country    string // (optional) country under which to report statistics
	bytesGiven int64  // tracks bytes given
}

// OnBytesGiven registers the fact that bytes were given (sent or received)
func (reporter *Reporter) OnBytesGiven(clientIp string, bytes int64) {
	atomic.AddInt64(&reporter.bytesGiven, bytes)
}

// reportStats periodically reports the stats to statshub via HTTP post
func (reporter *Reporter) Start() {
	for {
		nextInterval := time.Now().Truncate(REPORT_STATS_INTERVAL).Add(REPORT_STATS_INTERVAL)
		waitTime := nextInterval.Sub(time.Now())
		time.Sleep(waitTime)
		bytesGiven := atomic.SwapInt64(&reporter.bytesGiven, 0)
		err := reporter.postStats(bytesGiven)
		if err != nil {
			log.Errorf("Error on posting stats: %s", err)
		} else {
			log.Debugf("Reported %d bytesGiven to statshub", bytesGiven)
		}
	}
}

func (reporter *Reporter) postStats(bytesGiven int64) error {
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

	url := fmt.Sprintf(STATSHUB_URL_TEMPLATE, reporter.InstanceId)
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("Unable to post stats to statshub: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Unexpected response status posting stats to statshub: %d", resp.StatusCode)
	}
	return nil
}
