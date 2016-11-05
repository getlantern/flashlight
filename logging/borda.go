package logging

import (
	"net/http"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/proxybench"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var bordaClient *borda.Client

func createBordaClient(reportInterval time.Duration) {
	if bordaClient != nil {
		return
	}
	rt := proxied.ChainedThenFronted()
	bordaClient = borda.NewClient(&borda.Options{
		BatchInterval: reportInterval,
		Client: &http.Client{
			Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
				frontedURL := *req.URL
				frontedURL.Host = "d157vud77ygy87.cloudfront.net"
				op := ops.Begin("report_to_borda").Request(req)
				defer op.End()
				proxied.PrepareForFronting(req, frontedURL.String())
				return rt.RoundTrip(req)
			}),
		},
	})
}

func startBordaAndProxyBench(reportInterval time.Duration) {
	createBordaClient(reportInterval)
	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000, func(existingValues map[string]float64, newValues map[string]float64) {
		for key, value := range newValues {
			existingValues[key] += value
		}
	})

	proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {
		if isReportingEnabled() {
			reportToBorda(map[string]float64{"response_time": timing.Seconds()}, ctx)
		}
	})

	reporter := func(failure error, ctx map[string]interface{}) {
		if !isReportingEnabled() {
			return
		}

		cfg := getCfg()
		if !includeInSample(cfg.deviceID, cfg.bordaSamplePercentage) {
			return
		}

		values := map[string]float64{}
		if failure != nil {
			values["error_count"] = 1
		} else {
			values["success_count"] = 1
		}
		dialTime, found := ctx["dial_time"]
		if found {
			delete(ctx, "dial_time")
			values["dial_time"] = dialTime.(float64)
		}
		reportErr := reportToBorda(values, ctx)
		if reportErr != nil {
			log.Errorf("Error reporting error to borda: %v", reportErr)
		}
	}

	ops.RegisterReporter(reporter)
}
