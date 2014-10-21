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

type ClientReporter struct {
	Reporter
	traversalStats    map[string]*TraversalOutcome
	traversalOutcomes chan *TraversalOutcome
}

func (reporter *ClientReporter) Start() {
	reporter.traversalOutcomes = make(chan *TraversalOutcome)
	reporter.traversalStats = make(map[string]*TraversalOutcome)
	go reporter.coalesceTraversalStats()
}

// coalesceTraversalStats consolidates NAT traversal reporting
// timerCh is initially nil and we block until the
// first traversal happens; future traversals are coalesced
// until the timer is ready to fire.
// Once stats are reported, we return to the initial stat
func (reporter *ClientReporter) coalesceTraversalStats() {
	timer := time.NewTimer(CLIENT_INTERVAL)

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
		case <-timerCh:
			for answererCountry, outcome := range reporter.traversalStats {
				reporter.postTraversalStat(answererCountry, outcome)
			}
			reporter.traversalStats = make(map[string]*TraversalOutcome)
			timer.Reset(CLIENT_INTERVAL)
		}
	}
}

func (reporter *ClientReporter) postTraversalStat(answererCountry string, outcome *TraversalOutcome) error {
	report := map[string]interface{}{
		"dims": map[string]string{
			"answererCountry": answererCountry,
			"offererCountry":  reporter.Country,
			"operatingSystem": runtime.GOOS,
		},
		"increments": outcome,
	}
	jsonBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("Unable to decode traversal outcome: %s", err)
	}
	return reporter.postStats(jsonBytes)
}

func (reporter *ClientReporter) GetOutcomesCh() chan<- *TraversalOutcome {
	return reporter.traversalOutcomes
}
