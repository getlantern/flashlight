package geolookup

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/flashlight/proxied"
	"github.com/getlantern/fronted"
	"github.com/getlantern/libp2p/p2p"
	"github.com/stretchr/testify/require"
)

const initialInfo = `
{
    "City": {
        "City": {
            "GeoNameID": 4671654,
            "Names": {
                "de": "Austin",
                "en": "Austin",
                "es": "Austin",
                "fr": "Austin",
                "ja": "\u30aa\u30fc\u30b9\u30c6\u30a3\u30f3",
                "pt-BR": "Austin",
                "ru": "\u041e\u0441\u0442\u0438\u043d"
            }
        },
        "Continent": {
            "Code": "NA",
            "GeoNameID": 6255149,
            "Names": {
                "de": "Nordamerika",
                "en": "North America",
                "es": "Norteam\u00e9rica",
                "fr": "Am\u00e9rique du Nord",
                "ja": "\u5317\u30a2\u30e1\u30ea\u30ab",
                "pt-BR": "Am\u00e9rica do Norte",
                "ru": "\u0421\u0435\u0432\u0435\u0440\u043d\u0430\u044f \u0410\u043c\u0435\u0440\u0438\u043a\u0430",
                "zh-CN": "\u5317\u7f8e\u6d32"
            }
        },
        "Country": {
            "GeoNameID": 6252001,
            "IsoCode": "FM",
            "Names": {
                "de": "USA",
                "en": "United States",
                "es": "Estados Unidos",
                "fr": "\u00c9tats Unis",
                "ja": "\u30a2\u30e1\u30ea\u30ab",
                "pt-BR": "EUA",
                "ru": "\u0421\u0428\u0410",
                "zh-CN": "\u7f8e\u56fd"
            }
        },
        "Location": {
            "Latitude": 30.2095,
            "Longitude": -97.7972,
            "MetroCode": 635,
            "TimeZone": "America/Chicago"
        },
        "Postal": {
            "Code": "78745"
        },
        "RegisteredCountry": {
            "GeoNameID": 6252001,
            "IsoCode": "US",
            "Names": {
                "de": "USA",
                "en": "United States",
                "es": "Estados Unidos",
                "fr": "\u00c9tats Unis",
                "ja": "\u30a2\u30e1\u30ea\u30ab",
                "pt-BR": "EUA",
                "ru": "\u0421\u0428\u0410",
                "zh-CN": "\u7f8e\u56fd"
            }
        },
        "RepresentedCountry": {
            "GeoNameID": 0,
            "IsoCode": "",
            "Names": null,
            "Type": ""
        },
        "Subdivisions": [
            {
                "GeoNameID": 4736286,
                "IsoCode": "TX",
                "Names": {
                    "en": "Texas",
                    "es": "Texas",
                    "fr": "Texas",
                    "ja": "\u30c6\u30ad\u30b5\u30b9\u5dde",
                    "ru": "\u0422\u0435\u0445\u0430\u0441",
                    "zh-CN": "\u5fb7\u514b\u8428\u65af\u5dde"
                }
            }
        ],
        "Traits": {
            "IsAnonymousProxy": false,
            "IsSatelliteProvider": false
        }
    },
    "IP": "999.999.999.999"
}
`

func TestGetIP(t *testing.T) {
	currentGeoInfo = eventual.NewValue()
	roundTripper = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).Dial,
	}
	ip := GetIP(0)
	require.Equal(t, "", ip)
	go Refresh()
	ip = GetIP(-1)
	addr := net.ParseIP(ip)
	require.NotNil(t, addr)
}

func TestGetCountry(t *testing.T) {
	currentGeoInfo = eventual.NewValue()
	roundTripper = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).Dial,
	}

	country := GetCountry(0)
	require.Equal(t, "", country)
	go Refresh()
	country = GetCountry(-1)
	require.NotEmpty(t, country)
}

func TestFronted(t *testing.T) {
	currentGeoInfo = eventual.NewValue()
	geoFile, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	defer os.Remove(geoFile.Name())

	ioutil.WriteFile(geoFile.Name(), []byte(initialInfo), 0644)

	fronted.ConfigureHostAlaisesForTest(t, map[string]string{
		"geo.getiantem.org": "d3u5fqukq7qrhd.cloudfront.net",
	})

	// test persistence
	ch := OnRefresh()
	EnablePersistence(geoFile.Name())
	country := GetCountry(0)
	require.Equal(t, "FM", country, "Should immediately get persisted country")
	select {
	case <-ch:
		// okay
	case <-time.After(5 * time.Second):
		t.Error("should update watcher after enabling persistence")
	}

	// clear initial value to make sure we read value from network
	currentGeoInfo.Reset()
	Refresh()
	country = GetCountry(60 * time.Second)
	ip := GetIP(5 * time.Second)
	require.Len(t, country, 2, "Bad country '%v' for ip %v", country, ip)
	require.NotEqual(
		t,
		"FM",
		country,
		"Should have gotten a new country from network (note, this test will fail if run in Micronesia)",
	)
	require.True(t, len(ip) >= 7, "Bad IP %s", ip)

	select {
	case <-ch:
		// okay
	case <-time.After(5 * time.Second):
		t.Error("should update watcher after network refresh")
	}

	// Give persistence time to finish
	time.Sleep(1 * time.Second)
	b, err := ioutil.ReadFile(geoFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, b)
	require.NotEqual(
		t,
		initialInfo,
		string(b),
		"persisted geolocation information should have changed",
	)
}

func TestP2PGeolookup(t *testing.T) {
	freeP2pCtx, censoredP2pCtx := p2p.InitTestP2PPeers(t)
	t.Cleanup(func() {
		freeP2pCtx.Close(context.Background())
		censoredP2pCtx.Close(context.Background())
	})

	type testCase struct {
		Name                              string
		expectedSuccessfulFlowComponentID proxied.FlowComponentID
		onStartRoundTripFunc              proxied.OnStartRoundTrip
		onCompleteRoundTripFunc           proxied.OnCompleteRoundTrip
		init                              func(t *testing.T)
	}

	// This channel collects the winning roundtripper's ID.
	var winnerRoundTripperCh chan proxied.FlowComponentID

	for _, tc := range []testCase{
		// TODO <06-07-2022, soltzen> This test always fails since chained
		// doesn't work in tests. We'll have to mock it with a reverse proxy in
		// tests or a mock
		// {
		// 	Name: "Delay everything but chained",
		// 	onStartRoundTripFunc: func(id proxied.FlowComponentID, _ *http.Request) {
		// 		if id == proxied.FlowComponentID_P2P ||
		// 			id == proxied.FlowComponentID_Fronted {
		// 			// Delay forever
		// 			time.Sleep(9999 * time.Second)
		// 			return
		// 		}
		// 	},
		// 	onCompleteRoundTripFunc: func(id proxied.FlowComponentID) {
		// 		winnerRoundTripperCh <- id
		// 	},
		// 	expectedSuccessfulFlowComponentID: proxied.FlowComponentID_Chained,
		// },

		{
			Name: "Delay everything but fronted",
			init: func(t *testing.T) {
				// Configure fronted package
				fronted.ConfigureForTest(t)
				fronted.ConfigureHostAlaisesForTest(t, map[string]string{
					// XXX <31-01-22, soltzen> This API is a core component of Lantern and
					// will likely remain for a long time. It's safe to use for testing
					"geo.getiantem.org": "d3u5fqukq7qrhd.cloudfront.net",
				})
			},
			onStartRoundTripFunc: func(id proxied.FlowComponentID, _ *http.Request) {
				if id == proxied.FlowComponentID_P2P ||
					id == proxied.FlowComponentID_Chained {
					// Delay forever
					time.Sleep(9999 * time.Second)
					return
				}
			},
			onCompleteRoundTripFunc: func(id proxied.FlowComponentID) {
				winnerRoundTripperCh <- id
			},
			expectedSuccessfulFlowComponentID: proxied.FlowComponentID_Fronted,
		},

		{
			Name: "Delay everything but P2P",
			onStartRoundTripFunc: func(id proxied.FlowComponentID, _ *http.Request) {
				if id == proxied.FlowComponentID_Chained ||
					id == proxied.FlowComponentID_Fronted {
					// Delay forever
					time.Sleep(9999 * time.Second)
					return
				}
			},
			onCompleteRoundTripFunc: func(id proxied.FlowComponentID) {
				winnerRoundTripperCh <- id
			},
			expectedSuccessfulFlowComponentID: proxied.FlowComponentID_P2P,
		},
	} {
		// The "10" here is just arbitrary: this test is functional and deals
		// with a lot of different paths. It would be wise to run it multiple
		// times and make sure all of them succeed
		for i := 0; i < 10; i++ {
			// Init whatever we need to init for this case
			if tc.init != nil {
				tc.init(t)
			}

			// Reset the collection channel
			winnerRoundTripperCh = make(chan proxied.FlowComponentID, 3)
			// Reset the geolookup value
			currentGeoInfo = eventual.NewValue()

			// Set the roundtripper
			require.NoError(
				t,
				SetParallelFlowRoundTripper(
					censoredP2pCtx,
					5*time.Minute, // masqueradeTimeout
					true,          // addDebugHeaders
					tc.onStartRoundTripFunc,
					tc.onCompleteRoundTripFunc,
				),
			)

			// Refresh asynchronously and get the country
			go Refresh()
			country := GetCountry(5 * time.Second)
			require.NotEmpty(
				t,
				country,
				"This usually means we failed to do the geolookup in good time",
			)

			// If we succeeded, close the bufferred collection channel (so we
			// don't hang forever when receiving from the channel), assert only
			// **one** roundtripper succeeded, and it's the expected one
			close(winnerRoundTripperCh)
			require.Len(t, winnerRoundTripperCh, 1)
			require.Equal(
				t,
				tc.expectedSuccessfulFlowComponentID,
				<-winnerRoundTripperCh,
			)
		}
	}
}
