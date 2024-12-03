package issue

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/getlantern/fronted"
	tls "github.com/refraction-networking/utls"

	"github.com/getlantern/flashlight/v7/config"
	"github.com/getlantern/flashlight/v7/geolookup"
	"github.com/getlantern/flashlight/v7/proxied"
)

func TestMain(m *testing.M) {

	fronted := newFronted()
	proxied.SetFronted(fronted)

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
	fronted, err := fronted.NewFronted(filepath.Join(tempConfigDir, "masquerade_cache"), tls.HelloChrome_100, config.DefaultFrontedProviderID)
	if err != nil {
		log.Errorf("Unable to configure fronted: %v", err)
	}
	fronted.UpdateConfig(certs, cfg.Client.FrontedProviders())
	return fronted
}

func TestSendReport(t *testing.T) {
	fronted := newFronted()
	proxied.SetFronted(fronted)
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
	)
	if err != nil {
		t.Errorf("SendReport() error = %v", err)
	}
}
