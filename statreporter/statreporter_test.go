package statreporter

import (
	"testing"
	"time"

	"github.com/getlantern/testify/assert"
)

func TestAll(t *testing.T) {
	reportingPeriod := 200 * time.Millisecond
	instanceId := "testinstance"

	// Set up fake statshub
	reportCh := make(chan report)

	// Set up two dim groups with a dim in common and a dim different
	dg1 := Dim("a", "1").And("b", "1")
	dg2 := Dim("b", "2").And("a", "1")

	// Start reporting
	doStart(reportingPeriod, instanceId, "us", func(r report) error {
		go func() {
			reportCh <- r
		}()
		return nil
	})

	// Add stats
	dg1.Increment("incra").Add(1)
	dg1.Increment("incra").Add(1)
	dg1.Increment("incrb").Set(1)
	dg1.Increment("incrb").Set(25)
	dg1.Gauge("gaugea").Add(2)
	dg1.Gauge("gaugea").Add(2)
	dg1.Gauge("gaugeb").Set(2)
	dg1.Gauge("gaugeb").Set(48)

	dg2.Increment("incra").Add(1)
	dg2.Increment("incra").Add(1)
	dg2.Increment("incrb").Set(1)
	dg2.Increment("incrb").Set(25)
	dg2.Gauge("gaugea").Add(2)
	dg2.Gauge("gaugea").Add(2)
	dg2.Gauge("gaugeb").Set(2)
	dg2.Gauge("gaugeb").Set(48)

	expectedReport1 := report{
		"dims": map[string]string{
			"a": "1",
			"b": "1",
		},
		"increments": stats{
			"incra": 2,
			"incrb": 25,
		},
		"gauges": stats{
			"gaugea": 4,
			"gaugeb": 48,
		},
	}
	expectedReport2 := report{
		"dims": map[string]string{
			"a": "1",
			"b": "2",
		},
		"increments": stats{
			"incra": 2,
			"incrb": 25,
		},
		"gauges": stats{
			"gaugea": 4,
			"gaugeb": 48,
		},
	}

	report1 := <-reportCh
	report2 := <-reportCh

	// Since reports can be made in unpredictable order, figure out which one
	// is which
	if report1["dims"].(map[string]string)["b"] == "2" {
		// switch
		report1, report2 = report2, report1
	}

	assert.Equal(t, expectedReport1, report1, "1st report should equal expected")
	assert.Equal(t, expectedReport2, report2, "2nd report should equal expected")
}
