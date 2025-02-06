package issue

import (
	"os"
	"testing"
	"time"

	"github.com/getlantern/flashlight/v7/common"
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
