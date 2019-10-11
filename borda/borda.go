package borda

import (
	"context"
	"net/http"
	"sync"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/flashlight/proxied"
)

var (
	log = golog.LoggerFor("flashlight.borda")

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
	if common.InDevelopment() {
		log.Debug("In development, will report everything to borda every 1 minutes")
		reportInterval = 1 * time.Minute
		enabled = func(ctx map[string]interface{}) bool {
			return true
		}
	}

	origEnabled := enabled
	enabled = func(ctx map[string]interface{}) bool {
		return !common.InStealthMode() && origEnabled(ctx)
	}

	if reportInterval > 0 {
		log.Debug("Enabling borda")
		once.Do(func() {
			startBorda(reportInterval, enabled)
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

func startBorda(reportInterval time.Duration, enabled func(ctx map[string]interface{}) bool) {
	bordaClient = createBordaClient(reportInterval)

	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000)

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

		reportToBorda(values, ctx)
	}

	ops.RegisterReporter(reporter)
	golog.RegisterReporter(func(err error, severity golog.Severity, ctx map[string]interface{}) {
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
	rt.SetMasqueradeTimeout(30 * time.Second)
	return borda.NewClient(&borda.Options{
		BatchInterval: reportInterval,
		HTTPClient: &http.Client{
			Transport: proxied.AsRoundTripper(func(req *http.Request) (*http.Response, error) {
				op, ctx := ops.BeginWithNewBeam("report_to_borda", context.Background())
				op.Request(req)
				defer op.End()
				return rt.RoundTrip(req.WithContext(ctx))
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
