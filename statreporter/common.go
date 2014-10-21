package statreporter

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/getlantern/golog"
)

const (
	STATSHUB_URL_TEMPLATE = "https://pure-journey-3547.herokuapp.com/stats/%s"
)

var (
	log = golog.LoggerFor("flashlight.statreporter")
)

type Reporter struct {
	InstanceId string // (required) instanceid under which to report statistics
	Country    string // (optional) country under which to report statistics
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
