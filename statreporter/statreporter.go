package statreporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/nattywad"
)

const (
	STATSHUB_URL_TEMPLATE = "https://pure-journey-3547.herokuapp.com/stats/%s"
	REPORT_STATS_INTERVAL = 20 * time.Second
)

var (
	log = golog.LoggerFor("flashlight.nattest")
)

type TraversalStats []byte

type Reporter struct {
	InstanceId        string // (optional) instanceid under which to report statistics
	Country           string // (optional) country under which to report statistics
	OperatingSystem   string // operating system of client reporting stats
	bytesGiven        int64  // tracks bytes given
	traversalStats    TraversalStats
	TraversalOutcomes chan *nattywad.TraversalInfo
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
	reporter.TraversalOutcomes = make(chan *nattywad.TraversalInfo)
	go reporter.coalesceTraversalStats()
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

func (reporter *Reporter) convertTraversal(info *nattywad.TraversalInfo) (stat []byte) {

	answererCountry := ""
	if country, ok := info.Peer.Extras["country"]; ok {
		answererCountry = country.(string)
	}

	report := map[string]interface{}{
		"dims": map[string]string{
			"answererCountry": answererCountry,
			"offererCountry":  reporter.Country,
			"operatingSystem": "",
		},
		"increments": map[string]interface{}{
			"answererOnline":             info.ServerRespondedToSignaling,
			"answererGot5Tuple":          info.ServerGotFiveTuple,
			"offererGotFiveTuple":        info.OffererGotFiveTuple,
			"traversalSucceeded":         info.TraversalSucceeded,
			"connectionSucceeded":        info.ServerConnected,
			"durationOfSuccessTraversal": info.Duration,
		},
	}
	stat, err := json.Marshal(report)
	if err != nil {
		log.Errorf("Unable to marshal json for stats: %s", err)
	}
	return
}

// coalesceTraversalStats consolidates NAT traversal reporting
// timerCh is initially nil and we block until the
// first traversal happens; future traversals are coalesced
// until the timer is ready to fire.
// Once stats are reported, we return to the initial stat
func (reporter *Reporter) coalesceTraversalStats() {

	timer := time.NewTimer(0)

	var timerCh <-chan time.Time

	for {
		select {
		case outcome := <-reporter.TraversalOutcomes:
			stat := reporter.convertTraversal(outcome)
			log.Debugf("logging traversal stat %s", stat)
			reporter.traversalStats = append(reporter.traversalStats, stat...)
			if timerCh == nil {
				timer.Reset(10 * time.Second)
				timerCh = timer.C
			}
		case <-timerCh:
			reporter.postStats(reporter.traversalStats)
			reporter.traversalStats = []byte{}
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
