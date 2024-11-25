package issue

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/getlantern/fronted"

	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/geolookup"
)

func TestMain(m *testing.M) {

	newFronted()

	//log.Debug(cfg.Client.FrontedProviders())
	//fronted.Configure(certs, cfg.Client.FrontedProviders(), config.DefaultFrontedProviderID, filepath.Join(tempConfigDir, "masquerade_cache"))

	geolookup.GetCountry(1 * time.Minute)
	os.Exit(m.Run())
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
	fronted, err := fronted.NewFronted(certs, cfg.Client.FrontedProviders(), config.DefaultFrontedProviderID, filepath.Join(tempConfigDir, "masquerade_cache"))
	if err != nil {
		log.Errorf("Unable to configure fronted: %v", err)
	}
	return fronted
}

func TestSendReport(t *testing.T) {
	fronted := newFronted()
	err := sendReport(
		"34qsdf-24qsadf-32542q",
		"1",
		"token",
		"en",
		int(Request_NO_ACCESS),
		"Description placeholder",
		"pro",
		"jay+test@getlantern.org",
		"7.1.1",
		"Samsung Galaxy S10",
		"SM-G973F",
		"11",
		[]*Attachment{
			{
				Name: "Hello.txt",
				Data: []byte("Hello World"),
			},
		},
		fronted,
	)
	if err != nil {
		t.Errorf("SendReport() error = %v", err)
	}
}
