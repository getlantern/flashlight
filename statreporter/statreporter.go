package statreporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/getlantern/flashlight/log"
)

const (
	StatshubUrlTemplate = "https://%s/stats/%s"
)

type update struct {
	cat string
	key string
	val int64
}

type statsMap map[string]map[string]int64

type Reporter struct {
	ReportingInterval time.Duration
	StatshubAddr      string
	InstanceId        string // (optional) instanceid under which to report statistics
	Country           string // (optional) country under which to report statistics
	updates           chan *update
	stats             statsMap
}

func (reporter *Reporter) Increment(key string, value int64) {
	reporter.updates <- &update{"increments", key, value}
}

// OnBytesGiven registers the fact that bytes were given (sent or received)
func (reporter *Reporter) OnBytesGiven(clientIp string, bytes int64) {
	reporter.Increment("bytesGiven", bytes)
	reporter.Increment("bytesGivenByFlashlight", bytes)
}

// reportStats periodically coalesces the collected statistics and reports
// them to statshub via HTTP post
func (reporter *Reporter) Start() {
	reporter.updates = make(chan *update, 1000)
	reporter.stats = make(statsMap)

	timer := time.NewTimer(reporter.timeToNextReport())
	for {
		select {
		case update := <-reporter.updates:
			// Coalesce
			cat := reporter.stats[update.cat]
			if cat == nil {
				cat = make(map[string]int64)
				reporter.stats[update.cat] = cat
			}
			switch update.cat {
			case "increments":
				cat[update.key] = cat[update.key] + update.val
			default:
				log.Errorf("Received stat of unknown category: %s", update.cat)
			}
		case <-timer.C:
			if len(reporter.stats) == 0 {
				log.Debugf("No stats to report")
			} else {
				err := reporter.postStats(reporter.stats)
				if err != nil {
					log.Errorf("Error on posting stats: %s", err)
				}
				reporter.stats = make(statsMap)
			}
			timer.Reset(reporter.timeToNextReport())
		}
	}
}

func (reporter *Reporter) timeToNextReport() time.Duration {
	nextInterval := time.Now().Truncate(reporter.ReportingInterval).Add(reporter.ReportingInterval)
	return nextInterval.Sub(time.Now())
}

func (reporter *Reporter) postStats(stats map[string]map[string]int64) error {
	report := map[string]interface{}{
		"dims": map[string]string{
			"country": reporter.Country,
		},
	}
	for k, v := range stats {
		report[k] = v
	}

	jsonBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("Unable to marshal json for stats: %s", err)
	}

	url := fmt.Sprintf(StatshubUrlTemplate, reporter.StatshubAddr, reporter.InstanceId)
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("Unable to post stats to statshub: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Unexpected response status posting stats to statshub: %d", resp.StatusCode)
	}

	log.Debugf("Reported %s to statshub", string(jsonBytes))
	return nil
}
