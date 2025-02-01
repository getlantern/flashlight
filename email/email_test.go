package email

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/keighl/mandrill"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/golog"
)

var logger = golog.LoggerFor("email-test")

var tempConfigDir string

func TestMain(m *testing.M) {
	tempConfigDir, err := ioutil.TempDir("", "email_test")
	if err != nil {
		logger.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	os.Exit(m.Run())
}

func TestReadResponses(t *testing.T) {

	// Here are the various response statuses from
	// https://github.com/keighl/mandrill/blob/master/mandrill.go#L186
	// the sending status of the recipient - either "sent", "queued", "scheduled", "rejected", or "invalid"

	statuses := []string{
		"sent", "queued", "scheduled", "rejected", "invalid",
	}

	for _, status := range statuses {
		var responses []*mandrill.Response
		responses = append(responses, &mandrill.Response{Status: status})
		err := readResponses(responses)
		if status == "sent" || status == "queued" || status == "scheduled" {
			assert.Nil(t, err, "Expected no error for status "+status)
		} else if status == "rejected" || status == "invalid" {
			assert.False(t, err == nil)
		}
	}
}

func TestSubmitIssue(t *testing.T) {
	// Change the below to true if you want the test to submit a test email. To
	// test that domain-fronting is working, you can block mandrillapp.com, for
	// example by setting its address to 0.0.0.0 in /etc/hosts.
	if false {

		msg := &Message{
			To:       "ox+unittest@getlantern.org",
			From:     "ox+unittest@getlantern.org",
			Template: "user-send-logs-desktop",
		}
		assert.NoError(t, sendTemplate(context.Background(), msg), "Should be able to send email")
	}
}
