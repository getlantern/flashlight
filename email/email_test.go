package email

import (
	"testing"

	"github.com/keighl/mandrill"
	"github.com/stretchr/testify/assert"
)

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
