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
	StatshubUrlTemplate = "https://%s/stats/%s"
)

type increment struct {
	key string
	val int64
}

type average struct {
	key   string
	count int64
	total int64
}

func (avg *average) calc() int64 {
	return avg.total / avg.count
}

var (
	period     time.Duration
	addr       string
	id         string
	country    string
	incrCh     chan *increment
	avgCh      chan *average
	increments map[string]int64
	averages   map[string]*average
	started    int32
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
	country = countryCode
	incrCh = make(chan *increment, 1000)
	avgCh = make(chan *average, 1000)
	initAccumulators()

	timer := time.NewTimer(timeToNextReport())
	for {
		select {
		case next := <-incrCh:
			// Coalesce
			increments[next.key] = increments[next.key] + next.val
		case next := <-avgCh:
			// Coalesce
			existing := averages[next.key]
			if existing == nil {
				averages[next.key] = next
			} else {
				existing.count = existing.count + next.count
				existing.total = existing.total + next.total
			}
		case <-timer.C:
			if len(increments) == 0 && len(averages) == 0 {
				log.Debugf("No stats to report")
			} else {
				err := postStats(increments, averages)
				if err != nil {
					log.Errorf("Error on posting stats: %s", err)
				}
				initAccumulators()
			}
			timer.Reset(timeToNextReport())
		}
	}
}

func initAccumulators() {
	increments = make(map[string]int64)
	averages = make(map[string]*average)
}

func Increment(key string, value int64) {
	if isStarted() {
		incrCh <- &increment{key, value}
	}
}

func Average(key string, count int64, total int64) {
	if isStarted() {
		avgCh <- &average{key, count, total}
	}
}

func isStarted() bool {
	return atomic.LoadInt32(&started) == 1
}

// OnBytesGiven registers the fact that bytes were given (sent or received)
func OnBytesGiven(clientIp string, bytes int64) {
	Increment("bytesGiven", bytes)
	Increment("bytesGivenByFlashlight", bytes)
}

func timeToNextReport() time.Duration {
	nextInterval := time.Now().Truncate(period).Add(period)
	return nextInterval.Sub(time.Now())
}

func postStats(increments map[string]int64, averages map[string]*average) error {
	report := map[string]interface{}{
		"dims": map[string]string{
			"country": country,
		},
		"increments": increments,
	}

	gauges := make(map[string]int64)

	for key, avg := range averages {
		gauges[key] = avg.calc()
	}

	report["gauges"] = gauges

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
