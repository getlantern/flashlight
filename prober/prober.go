// prober is a utility program that will continuously probe a proxy
// configuration with a randomly varying pause between probes to exercise
// IdleTiming functionality. Call the program with a single argument pointing
// to a file that contains configuration YAML for a single proxy, like so:
//
// addr: 178.128.93.8:443
// cert: "-----BEGIN CERTIFICATE-----\nMIIDiTCCAnGgAwIBAgIJAK7xb7u2yR3/MA0GCSqGSIb3DQEBCwUAMGoxGzAZBgNV\nBAMTEllhbWFoYSBTdWJzdGFuZGFyZDEaMBgGA1UEChMRQmxhc3BoZW1pbmcgQWxw\naGExDTALBgNVBAcTBEdhbmcxEzARBgNVBAgTCk5ldyBKZXJzZXkxCzAJBgNVBAYT\nAlVTMB4XDTE4MTEyMDE5MzgyNFoXDTE5MTEyMDE5MzgyNFowajEbMBkGA1UEAxMS\nWWFtYWhhIFN1YnN0YW5kYXJkMRowGAYDVQQKExFCbGFzcGhlbWluZyBBbHBoYTEN\nMAsGA1UEBxMER2FuZzETMBEGA1UECBMKTmV3IEplcnNleTELMAkGA1UEBhMCVVMw\nggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDF0RWp9G3A5b6dR+TbP/vM\nyUwYb9IXzVxSBaFpsc13M3BztS6384dFAWkT6fHRp5r/Hs/cNx+0Oj/0bEC7GFGb\nPbs/S3BAD2i5qiz/ly9Qf1E82WeHYVgcIej35UvR1LJeYO8svgNMmhxtOOej9xoD\nov3mRwEaZicaKTYaNYab0LHLelLZZqRiJr8AuylsjE9EVzArbiNbm6PReZsEDssF\nKjjris3GxjTHF2+BHjd/GMTsB0tqvgf67dRahhY6XMG74HAcyFR7cDi2EzQOraAB\nJ3qyVjFKUTXhhb+Ra3eLaGqnFSwIYr5BLdwWfIZ0f8UakDTJ78Wy7xWFqzHKoh3H\nAgMBAAGjMjAwMB0GA1UdDgQWBBSLD84YzwP8ltOtMsmcRvV7loqJRTAPBgNVHREE\nCDAGhwSygF0IMA0GCSqGSIb3DQEBCwUAA4IBAQA6eRqyzZ4rDrd5BDLuxo9dbvtu\nJedcB61T877NRpO/3fRZrUH6Fgv+hl1C0WDIggYLPvAb74Q6D7AWX9mLFrsqQdUE\nqI373o2vNe9jLE+/5Xuphq6Thtsw94SjtSfukMBm7UmxGcuaooDAcg77R5b0i1pf\n4qc4dLOcVFjdlii3snCcZzj3gS62YCP7IdWNagYKiLXtPMlBNOL6Swi6eGbaLMHb\nQSSI8A3Dl4xcFkwpWTrYTUQiLJQF+URvpDoKF5QT4ux5v+eDeYmXP2tE5/HYsUH4\nhoy5jzWlcHCs7cq+Wl4LflFnXPceARg/SjvXQx40q+6Cq5R7YoIuK1DrR3iB\n-----END
//   CERTIFICATE-----\n"
// authtoken: wMJEBd1uUWeSakripkBYBiI5SGVn8685YjesoWrkml5jzxOj4Rk6tYMR3oFTEu4C
// trusted: true
// maxpreconnect: 0
// bias: 0
// pluggabletransport: lampshade
// pluggabletransportsettings: {}
// kcpsettings: {}
// enhttpurl: ""
// tlsdesktoporderedciphersuitenames: []
// tlsmobileorderedciphersuitenames: []
// tlsservernameindicator: ""
// tlsclientsessioncachesize: 0
// tlsclienthelloid: ""
// location:
//   city: Singapore
//   country: Singapore
//   countrycode: ""
//   latitude: 1.29
//   longitude: 103.86
// multiplexedaddr: ""
// multiplexedphysicalconns: 0

package main

import (
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/common"
)

var (
	log = golog.LoggerFor("prober")
)

func main() {
	chained.IdleTimeout = 2 * time.Second
	chained.PerformanceProbes = 1
	chained.BasePerformanceProbeKB = 100

	cfg, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("Unable to read config from %v: %v", os.Args[1], err)
	}
	server := &chained.ChainedServerInfo{}
	err = yaml.Unmarshal(cfg, server)
	if err != nil {
		log.Fatalf("Unable to read yaml config: %v", err)
	}

	dialer, err := chained.CreateDialer(os.Args[1], server, &common.UserConfigData{
		DeviceID: "XXXXXXXX",
		UserID:   0,
		Token:    "protoken",
	})
	if err != nil {
		log.Fatalf("Unable to create dialer: %v", err)
	}

	log.Debugf("Will probe %v", dialer.Label())
	for {
		if !dialer.Probe(true) {
			return
		}

		sleepTime := time.Duration(float64(chained.IdleTimeout) * 3 * rand.Float64())
		log.Debugf("Sleeping %v", sleepTime)
		time.Sleep(sleepTime)
	}
}
