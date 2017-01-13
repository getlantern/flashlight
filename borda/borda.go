package borda

import (
	"net/http"
	"sync"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxybench"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	log         = golog.LoggerFor("flashlight.borda")
	bordaClient *borda.Client
	once        sync.Once
)

// Configure starts borda reporting and proxy bench if reportInterval > 0. The
// service never stops once enabled. The service will check enabled each
// time before it reports to borda, however.
func Configure(reportInterval time.Duration, enabled func() bool) {
	if reportInterval > 0 {
		once.Do(func() {
			startBordaAndProxyBench(reportInterval, enabled)
		})
	}
}

func Close() {
	if bordaClient != nil {
		bordaClient.Flush()
	}
}

func startBordaAndProxyBench(reportInterval time.Duration, enabled func() bool) {
	bordaClient = createBordaClient(reportInterval)

	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000)

	proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {
		if enabled() {
			reportToBorda(map[string]borda.Val{"response_time": borda.Avg(timing.Seconds())}, ctx)
		}
	})

	reporter := func(failure error, ctx map[string]interface{}) {
		if !enabled() {
			return
		}

		values := map[string]borda.Val{}
		if failure != nil {
			values["error_count"] = borda.Float(1)
		} else {
			values["success_count"] = borda.Float(1)
		}

		// Convert metrics to values
		for dim, val := range ctx {
			metric, ok := val.(borda.Val)
			if ok {
				delete(ctx, dim)
				values[dim] = metric
			}
		}

		reportErr := reportToBorda(values, ctx)
		if reportErr != nil {
			log.Errorf("Error reporting error to borda: %v", reportErr)
		}
	}

	ops.RegisterReporter(reporter)
}

func createBordaClient(reportInterval time.Duration) *borda.Client {
	rt := proxied.ChainedThenFronted()
	return borda.NewClient(&borda.Options{
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

func metricToValue(dim string, ctx map[string]interface{}, values map[string]float64) {
	val, found := ctx[dim]
	if found {
		delete(ctx, dim)
		values[dim] = val.(float64)
	}
}
