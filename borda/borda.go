package borda

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxybench"

	"github.com/getlantern/flashlight/chained"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	log         = golog.LoggerFor("flashlight.borda")
	bordaClient *borda.Client
	tr          *trafficReporter
	once        sync.Once
)

// Configure starts borda reporting and proxy bench if reportInterval > 0. The
// service never stops once enabled. The service will check enabled each
// time before it reports to borda, however.
func Configure(deviceID string, reportInterval time.Duration, enabled func() bool) {
	if reportInterval > 0 {
		once.Do(func() {
			startBordaAndProxyBench(deviceID, reportInterval, enabled)
		})
	}
}

// Wraps a proxy to track traffic and report to borda if borda reporting
// enabled.
func WrapProxy(p chained.Proxy) chained.Proxy {
	if v := theProxyWrapper.Load(); v != nil {
		return v.(proxyWrapper)(p)
	}
	return p
}

func Close() {
	if tr != nil {
		tr.Stop()
	}
	if bordaClient != nil {
		bordaClient.Flush()
	}
}

type proxyWrapper func(chained.Proxy) chained.Proxy

var theProxyWrapper atomic.Value

func startBordaAndProxyBench(deviceID string, reportInterval time.Duration, enabled func() bool) {
	bordaClient = createBordaClient(reportInterval)
	var wrapper proxyWrapper
	tr, wrapper = newTrafficReporter(bordaClient, reportInterval, deviceID, enabled)
	theProxyWrapper.Store(wrapper)

	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000, func(existingValues map[string]float64, newValues map[string]float64) {
		for key, value := range newValues {
			existingValues[key] += value
		}
	})

	proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {
		if enabled() {
			reportToBorda(map[string]float64{"response_time": timing.Seconds()}, ctx)
		}
	})

	reporter := func(failure error, ctx map[string]interface{}) {
		if !enabled() {
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
