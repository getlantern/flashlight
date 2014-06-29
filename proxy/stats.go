package proxy

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

func (server *Server) onBytesGiven(clientIp string, bytes int64) {
	atomic.AddInt64(&server.bytesGiven, bytes)
}

func (server *Server) startReportingStatsIfNecessary() bool {
	if server.InstanceId != "" {
		log.Debugf("Reporting stats under InstanceId %s", server.InstanceId)
		go server.reportStats()
		return true
	} else {
		log.Debug("Not reporting stats (no InstanceId specified)")
		return false
	}
}

// reportStats periodically checkpoints the total bytes given and reports them
// to statshub via HTTP post
func (server *Server) reportStats() {
	for {
		nextInterval := time.Now().Truncate(REPORT_STATS_INTERVAL).Add(REPORT_STATS_INTERVAL)
		waitTime := nextInterval.Sub(time.Now())
		time.Sleep(waitTime)
		bytesGiven := atomic.SwapInt64(&server.bytesGiven, 0)
		err := server.postStats(bytesGiven)
		if err != nil {
			log.Errorf("Error on posting stats: %s", err)
		} else {
			log.Debugf("Reported %d bytesGiven to statshub", bytesGiven)
		}
	}
}

func (server *Server) postStats(bytesGiven int64) error {
	report := map[string]interface{}{
		"dims": map[string]string{
			"country": server.Country,
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

	url := fmt.Sprintf(STATSHUB_URL_TEMPLATE, server.InstanceId)
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
