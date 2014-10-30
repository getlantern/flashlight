package statreporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("statreporter")
)

const (
	StatshubUrlTemplate = "https://%s/stats/%s"
)

type stats map[string]int64

type dimAccumulator struct {
	dims       *Dims
	categories map[string]stats
}

var (
	Country      string
	period       time.Duration
	addr         string
	id           string
	updatesCh    chan *update
	accumulators map[string]*dimAccumulator
	started      int32
)

// Start runs a goroutine that periodically coalesces the collected statistics
// and reports them to statshub via HTTP post
func Start(reportingPeriod time.Duration, statshubAddr string, instanceId string, countryCode string) {
	alreadyStarted := !atomic.CompareAndSwapInt32(&started, 0, 1)
	if alreadyStarted {
		log.Debugf("statreporter already started, not starting again")
		return
	}
	period = reportingPeriod
	addr = statshubAddr
	id = instanceId
	Country = strings.ToLower(countryCode)
	updatesCh = make(chan *update, 1000)
	accumulators = make(map[string]*dimAccumulator)

	timer := time.NewTimer(timeToNextReport())
	for {
		select {
		case update := <-updatesCh:
			// Coalesce
			dimsKey := update.dims.String()
			dimAccum := accumulators[dimsKey]
			if dimAccum == nil {
				dimAccum = &dimAccumulator{
					dims:       update.dims,
					categories: make(map[string]stats),
				}
				accumulators[dimsKey] = dimAccum
			}
			categoryStats := dimAccum.categories[update.category]
			if categoryStats == nil {
				categoryStats = make(stats)
				dimAccum.categories[update.category] = categoryStats
			}
			switch update.action {
			case set:
				categoryStats[update.key] = update.val
			case add:
				categoryStats[update.key] = categoryStats[update.key] + update.val
			}
		case <-timer.C:
			if len(accumulators) == 0 {
				log.Debugf("No stats to report")
			} else {
				postStats(accumulators)
				accumulators = make(map[string]*dimAccumulator)
			}
			timer.Reset(timeToNextReport())
		}
	}
}

func postUpdate(update *update) {
	if isStarted() {
		updatesCh <- update
	}
}

func isStarted() bool {
	return atomic.LoadInt32(&started) == 1
}

func timeToNextReport() time.Duration {
	nextInterval := time.Now().Truncate(period).Add(period)
	return nextInterval.Sub(time.Now())
}

func postStats(accumulators map[string]*dimAccumulator) {
	for _, dimAccum := range accumulators {
		dimsMap := make(map[string]string)
		for i, key := range dimAccum.dims.keys {
			dimsMap[key] = dimAccum.dims.values[i]
		}
		err := postStatsForDims(dimsMap, dimAccum)
		if err != nil {
			log.Errorf("Unable to post stats for dim %s: %s", err)
		}
	}
}

func postStatsForDims(dimsMap map[string]string, dimAccum *dimAccumulator) error {
	report := map[string]interface{}{
		"dims": dimsMap,
	}

	for category, accum := range dimAccum.categories {
		report[category] = accum
	}

	jsonBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("Unable to marshal json for stats: %s", err)
	}

	url := fmt.Sprintf(StatshubUrlTemplate, addr, id)
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
