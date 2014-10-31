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

type dimGroupAccumulator struct {
	dg         *DimGroup
	categories map[string]stats
}

var (
	Country      string
	period       time.Duration
	id           string
	updatesCh    chan *update
	accumulators map[string]*dimGroupAccumulator
	started      int32
)

type report map[string]interface{}

type reportPoster func(report report) error

// Start runs a goroutine that periodically coalesces the collected statistics
// and reports them to statshub via HTTPS post
func Start(reportingPeriod time.Duration, statshubAddr string, instanceId string, countryCode string) {
	doStart(reportingPeriod, instanceId, countryCode, posterForDimGroupStats(statshubAddr))
}

func doStart(reportingPeriod time.Duration, instanceId string, countryCode string, postReport reportPoster) {
	alreadyStarted := !atomic.CompareAndSwapInt32(&started, 0, 1)
	if alreadyStarted {
		log.Debugf("statreporter already started, not starting again")
		return
	}

	period = reportingPeriod
	id = instanceId
	Country = strings.ToLower(countryCode)
	// We buffer the updates channel to be able to continue accepting updates while we're posting a report
	updatesCh = make(chan *update, 1000)
	accumulators = make(map[string]*dimGroupAccumulator)

	timer := time.NewTimer(timeToNextReport())
	go func() {
		for {
			select {
			case update := <-updatesCh:
				log.Tracef("Coalescing update: %s", update)
				// Coalesce
				dgKey := update.dg.String()
				dgAccum := accumulators[dgKey]
				if dgAccum == nil {
					dgAccum = &dimGroupAccumulator{
						dg:         update.dg,
						categories: make(map[string]stats),
					}
					accumulators[dgKey] = dgAccum
				}
				categoryStats := dgAccum.categories[update.category]
				if categoryStats == nil {
					categoryStats = make(stats)
					dgAccum.categories[update.category] = categoryStats
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
					postStats(accumulators, postReport)
					accumulators = make(map[string]*dimGroupAccumulator)
				}
				timer.Reset(timeToNextReport())
			}
		}
	}()
}

func postUpdate(update *update) {
	if isStarted() {
		select {
		case updatesCh <- update:
			// update posted
		default:
			// drop stat to avoid blocking
		}
	}
}

func isStarted() bool {
	return atomic.LoadInt32(&started) == 1
}

func timeToNextReport() time.Duration {
	nextInterval := time.Now().Truncate(period).Add(period)
	return nextInterval.Sub(time.Now())
}

func postStats(accumulators map[string]*dimGroupAccumulator, postReport reportPoster) {
	for _, dgAccum := range accumulators {
		err := postReport(dgAccum.report())
		if err != nil {
			log.Errorf("Unable to post stats for dim %s: %s", err)
		}
	}
}

func posterForDimGroupStats(addr string) reportPoster {
	return func(report report) error {
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
}

func (dgAccum *dimGroupAccumulator) report() report {
	report := report{
		"dims": dgAccum.dg.dims,
	}

	for category, accum := range dgAccum.categories {
		report[category] = accum
	}

	return report
}
