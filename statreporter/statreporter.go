package statreporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
)

const (
	STATSHUB_URL_TEMPLATE      = "https://pure-journey-3547.herokuapp.com/stats/%s"
	REPORT_STATS_INTERVAL      = 20 * time.Second
	REPORT_TRAVERSALS_INTERVAL = 5 * time.Minute
)

var (
	log = golog.LoggerFor("flashlight.statreporter")
)

type TraversalOutcome struct {
	AnswererCountry               string        `json:"-"`
	AnswererOnline                int           `json:"answererOnline"`
	AnswererGot5Tuple             int           `json:"answererGotFiveTuple"`
	OffererGot5Tuple              int           `json:"offererGotFiveTuple"`
	TraversalSucceeded            int           `json:"traversalSucceeded"`
	ConnectionSucceeded           int           `json:"connectionSucceeded"`
	DurationOfSuccessfulTraversal time.Duration `json:"durationOfTraversal"`
}

func (o *TraversalOutcome) merge(n *TraversalOutcome) {
	o.AnswererOnline = o.AnswererOnline + n.AnswererOnline
	o.AnswererGot5Tuple = o.AnswererGot5Tuple + n.AnswererGot5Tuple
	o.OffererGot5Tuple = o.OffererGot5Tuple + n.OffererGot5Tuple
	o.ConnectionSucceeded = o.ConnectionSucceeded + n.ConnectionSucceeded
	o.TraversalSucceeded = o.TraversalSucceeded + n.TraversalSucceeded
	o.ConnectionSucceeded = o.ConnectionSucceeded + n.ConnectionSucceeded
	o.DurationOfSuccessfulTraversal = o.DurationOfSuccessfulTraversal + n.DurationOfSuccessfulTraversal
}

type Reporter struct {
	InstanceId        string // (optional) instanceid under which to report statistics
	Country           string // (optional) country under which to report statistics
	bytesGiven        int64  // tracks bytes given
	traversalStats    map[string]*TraversalOutcome
	traversalOutcomes chan *TraversalOutcome
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
		err := reporter.postGiveStats(bytesGiven)
		if err != nil {
			log.Errorf("Error on posting stats: %s", err)
		} else {
			log.Debugf("Reported %d bytesGiven to statshub", bytesGiven)
		}
	}
}

func (reporter *Reporter) ListenForTraversals() {
	reporter.traversalOutcomes = make(chan *TraversalOutcome)
	reporter.traversalStats = make(map[string]*TraversalOutcome)
	go reporter.coalesceTraversalStats()
}

func (reporter *Reporter) GetOutcomesCh() chan<- *TraversalOutcome {
	return reporter.traversalOutcomes
}

func (reporter *Reporter) postStats(jsonBytes []byte) error {
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

func (reporter *Reporter) postTraversalStat(answererCountry string, outcome *TraversalOutcome) error {

	var buffer bytes.Buffer
	enc := json.NewEncoder(&buffer)

	report := map[string]interface{}{
		"dims": map[string]string{
			"answererCountry": answererCountry,
			"offererCountry":  reporter.Country,
			"operatingSystem": runtime.GOOS,
		},
		"increments": outcome,
	}
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("Unable to decode traversal outcome: %s", err)
	}
	return reporter.postStats(buffer.Bytes())
}

// coalesceTraversalStats consolidates NAT traversal reporting
// timerCh is initially nil and we block until the
// first traversal happens; future traversals are coalesced
// until the timer is ready to fire.
// Once stats are reported, we return to the initial stat
func (reporter *Reporter) coalesceTraversalStats() {

	timer := time.NewTimer(REPORT_STATS_INTERVAL)

	var timerCh <-chan time.Time

	for {
		select {
		case n := <-reporter.traversalOutcomes:
			o := reporter.traversalStats[n.AnswererCountry]
			if o == nil {
				reporter.traversalStats[n.AnswererCountry] = n
			} else {
				o.merge(n)
			}
			if timerCh == nil {
				timer.Reset(REPORT_TRAVERSALS_INTERVAL)
				timerCh = timer.C
			}
		case <-timerCh:
			for answererCountry, outcome := range reporter.traversalStats {
				reporter.postTraversalStat(answererCountry, outcome)
			}
			reporter.traversalStats = make(map[string]*TraversalOutcome)
			timer.Reset(REPORT_TRAVERSALS_INTERVAL)
		}
	}
}

func (reporter *Reporter) postGiveStats(bytesGiven int64) error {
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
