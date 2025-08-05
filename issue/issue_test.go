package issue

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlantern/flashlight/v7/common"
	userconfig "github.com/getlantern/flashlight/v7/config/user"
	"github.com/getlantern/flashlight/v7/geolookup"
)

func TestMain(m *testing.M) {

	//log.Debug(cfg.Client.FrontedProviders())
	//fronted.Configure(certs, cfg.Client.FrontedProviders(), config.DefaultFrontedProviderID, filepath.Join(tempConfigDir, "masquerade_cache"))

	geolookup.GetCountry(1 * time.Minute)
	os.Exit(m.Run())
}

func TestSendReport(t *testing.T) {
	//manually set library version since its only populated when run from a binary
	common.LibraryVersion = "7.0.0"
	UserConfigData := common.UserConfigData{}

	closeTestFiles := initTestConfig(t)
	defer closeTestFiles()

	err := sendReport(
		&UserConfigData,
		"34qsdf-24qsadf-32542q",
		"1",
		"token",
		"en",
		int(Request_NO_ACCESS),
		"Description placeholder",
		"pro",
		"thomas+flashlighttest@getlantern.org",
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
		"US",
	)
	if err != nil {
		t.Errorf("SendReport() error = %v", err)
	}
}

func initTestConfig(t *testing.T) func() {
	dir := "testdata"
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Logf("Failed to create testdata dir: %v", err)
	}

	f, err := os.Create(filepath.Join(dir, "user.conf"))
	if err != nil {
		t.Logf("Failed to create temp file: %v", err)
	}

	f.Write([]byte(`{
	  "country": "US",
	  "proxy": {
	    "proxies": [
	      {
	        "addr": "1.1.1.1",
	        "track": "raichu",
	        "location": {
	          "city": "New York",
	          "country": "United States",
	          "countryCode": "US",
	          "latitude": 1.23,
	          "longitude": 4.56
	        },
	        "name": "raichu-proxy",
	        "port": 123,
	        "protocol": "tlsmasq",
	        "certPem": "pem",
	        "authToken": "token",
	        "trusted": true,
	        "connectCfgTlsmasq": {
	          "originAddr": "google.com",
	          "secret": "password",
	          "tlsMinVersion": "1"
	        }
	      }
	    ]
	  }
	}`))

	conf, err := userconfig.Init(dir, true)
	if err != nil {
		t.Logf("Failed to initialize user config: %v", err)
	}
	if conf != nil {
		t.Logf("User config initialized successfully")
	}
	return func() {
		f.Close()
		os.RemoveAll(dir)
	}
}
