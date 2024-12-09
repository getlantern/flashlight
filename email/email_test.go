package email

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/keighl/mandrill"
	tls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/assert"

	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/getlantern/fronted"
	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"
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
		proxied.SetFronted(newFronted())

		msg := &Message{
			To:       "ox+unittest@getlantern.org",
			From:     "ox+unittest@getlantern.org",
			Template: "user-send-logs-desktop",
		}
		assert.NoError(t, sendTemplate(context.Background(), msg), "Should be able to send email")
	}
}

func newFronted() fronted.Fronted {
	// Init domain-fronting
	global, err := os.ReadFile("../embeddedconfig/global.yaml")
	if err != nil {
		log.Errorf("Unable to load embedded global config: %v", err)
		os.Exit(1)
	}
	cfg := config.NewGlobal()
	err = yaml.Unmarshal(global, cfg)
	if err != nil {
		log.Errorf("Unable to unmarshal embedded global config: %v", err)
		os.Exit(1)
	}

	certs, err := cfg.TrustedCACerts()
	if err != nil {
		log.Errorf("Unable to read trusted certs: %v", err)
	}

	tempConfigDir, err := os.MkdirTemp("", "issue_test")
	if err != nil {
		log.Errorf("Unable to create temp config dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempConfigDir)
	fronted, err := fronted.NewFronted(filepath.Join(tempConfigDir, "masquerade_cache"), tls.HelloChrome_100, config.DefaultFrontedProviderID)
	if err != nil {
		log.Errorf("Unable to configure fronted: %v", err)
	}
	fronted.UpdateConfig(certs, cfg.Client.FrontedProviders())
	return fronted
}
