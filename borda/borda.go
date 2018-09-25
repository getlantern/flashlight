package borda

import (
	"net/http"
	"sync"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/errors"
	"github.com/getlantern/proxybench"
	"github.com/getlantern/zaplog"

	"github.com/getlantern/flashlight/logging"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"

	"go.uber.org/zap/zapcore"
)

var (
	log = logging.LoggerFor("flashlight.borda")

	// BeforeSubmit is an optional callback to capture when borda batches are
	// submitted. It's mostly useful for unit testing.
	BeforeSubmit func(name string, ts time.Time, values map[string]borda.Val, dimensionsJSON []byte)

	bordaClient *borda.Client

	once sync.Once
)

// Configure starts borda reporting and proxy bench if reportInterval > 0. The
// service never stops once enabled. The service will check enabled each
// time before it reports to borda, however.
func Configure(reportInterval time.Duration, enabled func(ctx map[string]interface{}) bool) {
	if reportInterval > 0 {
		log.Info("Enabling borda")
		once.Do(func() {
			startBordaAndProxyBench(reportInterval, enabled)
		})
	} else {
		log.Info("Borda not enabled")
	}
}

// Flush flushes any pending submission
func Flush() {
	if bordaClient != nil {
		log.Infof("Flushing pending borda submissions")
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

	zaplog.AddWarnHook(func(entry zapcore.Entry) error {
		ctx := make(map[string]interface{})
		ctxErr := errors.New(entry.Message)
		ctxErr.Fill(ctx)
		flushImmediately := false
		op := "catchall"
		if entry.Level == zapcore.FatalLevel {
			op = op + "_fatal"
			flushImmediately = true
		}
		ctx["op"] = op
		reporter(ctxErr, ctx)
		if flushImmediately {
			Flush()
		}
		return nil
	})
}

func createBordaClient(reportInterval time.Duration) *borda.Client {
	rt := proxied.ChainedThenFronted()
	rt.SetMasqueradeTimeout(30 * time.Second)
	return borda.NewClient(&borda.Options{
		BatchInterval: reportInterval,
		HTTPClient: &http.Client{
			Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
				op := ops.Begin("report_to_borda").Request(req)
				defer op.End()
				return rt.RoundTrip(req)
			}),
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
