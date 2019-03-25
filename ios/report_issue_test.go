package ios

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReportIssue(t *testing.T) {
	assert.NoError(t, ReportIssue("0.0.1", "TestMachine", "1.0", "ox@getlantern.org", "This is just a test", "testlogs/", "testlogs/"))
}
