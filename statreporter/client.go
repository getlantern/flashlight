package statreporter

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

var (
	CLIENT_INTERVAL = 5 * time.Minute
)

type TraversalOutcome struct {
	AnswererCountry     string `json:"-"`
	AnswererOnline      uint64 `json:"answererOnline"`
	AnswererGot5Tuple   uint64 `json:"answererGotFiveTuple"`
	OffererGot5Tuple    uint64 `json:"offererGotFiveTuple"`
	TraversalSucceeded  uint64 `json:"traversalSucceeded"`
	ConnectionSucceeded uint64 `json:"connectionSucceeded"`

	// DurationOfSuccessfulTraversal is the duration in seconds
	DurationOfSuccessfulTraversal uint64 `json:"durationOfTraversal"`
}

func (o *TraversalOutcome) coalesce(n *TraversalOutcome) {
	o.AnswererOnline = o.AnswererOnline + n.AnswererOnline
	o.AnswererGot5Tuple = o.AnswererGot5Tuple + n.AnswererGot5Tuple
	o.OffererGot5Tuple = o.OffererGot5Tuple + n.OffererGot5Tuple
	o.ConnectionSucceeded = o.ConnectionSucceeded + n.ConnectionSucceeded
	o.TraversalSucceeded = o.TraversalSucceeded + n.TraversalSucceeded
	o.ConnectionSucceeded = o.ConnectionSucceeded + n.ConnectionSucceeded
	o.DurationOfSuccessfulTraversal = o.DurationOfSuccessfulTraversal + n.DurationOfSuccessfulTraversal
}

type ClientReporter struct {
	Reporter
	OutcomesCh     chan<- *TraversalOutcome
	outcomesCh     chan *TraversalOutcome
	traversalStats map[string]*TraversalOutcome
}

func (reporter *ClientReporter) Start() {
	reporter.outcomesCh = make(chan *TraversalOutcome, 100)
	reporter.OutcomesCh = reporter.outcomesCh
	reporter.traversalStats = make(map[string]*TraversalOutcome)
	go reporter.processTraversalStats()
}

// processTraversalStats coalesces TraversalOutcomes as they are received and
// periodically reports these to statshub. The first  TraversalOutcome is
// reported immediately, after which we reported coalesced outcomes every 5
// minutes.
func (reporter *ClientReporter) processTraversalStats() {
	// Immediately post the first available stat
	n := <-reporter.outcomesCh
	reporter.postTraversalStats(n)

	// Then start coalescing stats and posting every CLIENT_INTERVAL
	timer := time.NewTimer(CLIENT_INTERVAL)
	for {
		select {
		case n := <-reporter.outcomesCh:
			o := reporter.traversalStats[n.AnswererCountry]
			if o == nil {
				reporter.traversalStats[n.AnswererCountry] = n
			} else {
				o.coalesce(n)
			}
		case <-timer.C:
			for _, outcome := range reporter.traversalStats {
				reporter.postTraversalStats(outcome)
			}
			reporter.traversalStats = make(map[string]*TraversalOutcome)
			timer.Reset(CLIENT_INTERVAL)
		}
	}
}

func (reporter *ClientReporter) postTraversalStats(outcome *TraversalOutcome) error {
	report := map[string]interface{}{
		"dims": map[string]string{
			"answererCountry": outcome.AnswererCountry,
			"offererCountry":  reporter.Country,
			"operatingSystem": runtime.GOOS,
		},
		"increments": outcome,
	}
	jsonBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("Unable to decode traversal outcome: %s", err)
	}
	log.Tracef("Reporting: %s", string(jsonBytes))
	return reporter.postStats(jsonBytes)
}
