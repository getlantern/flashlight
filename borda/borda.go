package borda

import (
	"net/http"
	"sync"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/proxybench"

	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	log = golog.LoggerFor("flashlight.borda")

	// BeforeSubmit is an optional callback to capture when borda batches are
	// submitted. It's mostly useful for unit testing.
	BeforeSubmit func(name string, key string, ts time.Time, values map[string]borda.Val, dimensions map[string]interface{})

	bordaClient *borda.Client

	once sync.Once
)

// Configure starts borda reporting and proxy bench if reportInterval > 0. The
// service never stops once enabled. The service will check enabled each
// time before it reports to borda, however.
func Configure(reportInterval time.Duration, enabled func(ctx map[string]interface{}) bool) {
	if reportInterval > 0 {
		log.Debug("Enabling borda")
		once.Do(func() {
			startBordaAndProxyBench(reportInterval, enabled)
		})
	} else {
		log.Debug("Borda not enabled")
	}
}

// Flush flushes any pending submission
func Flush() {
	if bordaClient != nil {
		log.Debugf("Flushing pending borda submissions")
		bordaClient.Flush()
	}
}

func startBordaAndProxyBench(reportInterval time.Duration, enabled func(ctx map[string]interface{}) bool) {
	bordaClient = createBordaClient(reportInterval)

	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000)

	proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {
		// No need to do anything, this is now handled with the regular op reporting
	})

	reporter := func(failure error, ctx map[string]interface{}) {
		if !enabled(ctx) {
			return
		}

		values := map[string]borda.Val{}
		if failure != nil {
			values["error_count"] = borda.Sum(1)
		} else {
			values["success_count"] = borda.Sum(1)
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
	golog.RegisterReporter(func(err error, linePrefix string, severity golog.Severity, ctx map[string]interface{}) {
		// This code catches all logged errors that didn't happen inside an op
		if ctx["op"] == nil {
			var ctxErr errors.Error
			switch e := err.(type) {
			case errors.Error:
				ctxErr = e
			case error:
				ctxErr = errors.Wrap(e)
			default:
				ctxErr = errors.New(e.Error())
			}

			ctxErr.Fill(ctx)
			flushImmediately := false
			op := "catchall"
			if severity == golog.FATAL {
				op = op + "_fatal"
				flushImmediately = true
			}
			ctx["op"] = op
			reporter(err, ctx)
			if flushImmediately {
				Flush()
			}
		}
	})
}

func createBordaClient(reportInterval time.Duration) *borda.Client {
	rt := proxied.ChainedThenFronted()
	return borda.NewClient(&borda.Options{
		BatchInterval: reportInterval,
		HTTPClient: &http.Client{
			Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
				frontedURL := *req.URL
				frontedURL.Host = "d157vud77ygy87.cloudfront.net"
				op := ops.Begin("report_to_borda").Request(req)
				defer op.End()
				proxied.PrepareForFronting(req, frontedURL.String())
				return rt.RoundTrip(req)
			}),
			Timeout: time.Second * 10,
		},
		BeforeSubmit: BeforeSubmit,
	})
}

func metricToValue(dim string, ctx map[string]interface{}, values map[string]float64) {
	val, found := ctx[dim]
	if found {
		delete(ctx, dim)
		values[dim] = val.(float64)
	}
}
