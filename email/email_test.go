package email

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/config/generated"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/fronted"
	"github.com/getlantern/keyman"
	"github.com/getlantern/yaml"
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

func TestSubmitIssue(t *testing.T) {
	// Change the below to true if you want the test to submit a test email. To
	// test that domain-fronting is working, you can block mandrillapp.com, for
	// example by setting its address to 0.0.0.0 in /etc/hosts.
	if false {
		cfg := &config.Global{}
		err := yaml.Unmarshal(generated.GlobalConfig, cfg)
		if !assert.NoError(t, err) {
			return
		}

		certs := make([]string, 0, len(cfg.TrustedCAs))
		for _, ca := range cfg.TrustedCAs {
			certs = append(certs, ca.Cert)
		}
		pool, err := keyman.PoolContainingCerts(certs...)
		if !assert.NoError(t, err) {
			return
		}

		fronted.Configure(pool, cfg.Client.FrontedProviders(), config.CloudfrontProviderID, filepath.Join(appdir.General("Lantern"), "masquerade_cache"))
		SetHTTPClient(proxied.DirectThenFrontedClient(5 * time.Second))
		defer SetHTTPClient(&http.Client{})

		msg := &Message{
			To:       "ox+unittest@getlantern.org",
			From:     "ox+unittest@getlantern.org",
			Template: "user-send-logs-desktop",
		}
		assert.NoError(t, sendTemplate(msg), "Should be able to send email")
	}
}
