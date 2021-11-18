package borda

import (
	"context"
	"math/rand"
	"net/http"
	"regexp"
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

	// FullyReportedOps are ops which are reported at 100% to borda, irrespective
	// of the borda sample percentage. This should all be low-volume operations,
	// otherwise we will utilize too much bandwidth on the client.
	FullyReportedOps = []string{
		"proxybench",
		"client_started",
		"client_stopped",
		"connect",
		"disconnect",
		"traffic",
		"catchall_fatal",
		"sysproxy_on",
		"sysproxy_off",
		"sysproxy_off_force",
		"sysproxy_clear",
		"report_issue",
		"proxy_rank",
		"proxy_selection_stability",
		"probe",
		"dial_for_balancer",
		"proxy_dial",
		"youtube_view",
		"replica_upload",
		"replica_view",
		"replica_torrent_peer_sent_data",
		"install_mitm_cert"}

	// LightweightOps are ops for which we record less than the full set of dimensions (e.g. omit error)
	LightweightOps = []string{
		"dial_for_balancer",
		"proxy_dial",
		"youtube_view"}

	// BeforeSubmit is an optional callback to capture when borda batches are
	// submitted. It's mostly useful for unit testing.
	BeforeSubmit func(name string, ts time.Time, values map[string]borda.Val, dimensionsJSON []byte)

	// Set by SetURL.
	// n.b. If SetURL is never called, the empty string is provided to getlantern/borda, resulting
	// in use of getlantern/borda.DefaultURL.
	bordaURL   string
	bordaURLMx sync.Mutex

	bordaClient   *borda.Client
	bordaClientMx sync.Mutex

	// Overrides any configured report interval. Intended for use only by tests.
	overrideReportInterval   time.Duration
	overrideReportIntervalMx sync.Mutex

	once sync.Once

	// Regexp based on code from https://codverter.com/blog/articles/tech/20190105-extract-ipv4-ipv6-ip-addresses-using-regex.html
	sanitizeAddrsRegex = regexp.MustCompile(`(((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}))|:)))(%.+)?|(\d{1,3}\.){3}\d{1,3})(:[0-9]+)?`)
)

// EnabledFunc is a function that indicates whether reporting to borda is enabled for a specific context.
type EnabledFunc func(ctx map[string]interface{}) bool

// Enabler creates an EnabledFunc for a given sample percentage
func Enabler(samplePercentage float64) EnabledFunc {
	return func(ctx map[string]interface{}) bool {
		if rand.Float64() <= samplePercentage/100 {
			return true
		}

		// For some ops, we don't randomly sample, we include all of them
		_op := ctx["op"]
		op, ok := _op.(string)
		if ok {
			for _, fullyReportedOp := range FullyReportedOps {
				if op == fullyReportedOp {
					log.Tracef("Including fully reported op %v in borda sample", fullyReportedOp)
					return true
				}
			}
		}
		return false
	}
}

// SetURL sets the URL which should be used to reach Borda. This is mostly intended for use by
// tests. Must be called before the first call to Configure (which may be called at any time once
// flashlight.Flashlight.Run is called). Once Configure is called, the URL is set.
func SetURL(url string) {
	bordaURLMx.Lock()
	bordaURL = url
	bordaURLMx.Unlock()
}

// SetOverrideReportIntervalForTesting sets a report interval which overrides any interval provided
// to Configure. This is only intended for use by tests.
func SetOverrideReportIntervalForTesting(reportInterval time.Duration) {
	overrideReportIntervalMx.Lock()
	overrideReportInterval = reportInterval
	overrideReportIntervalMx.Unlock()
}

// Configure starts borda reporting if reportInterval > 0. The service never stops once enabled.
// The service will check enabled each time before it reports to borda, however.
func Configure(reportInterval time.Duration, enabled EnabledFunc) {
	log.Debugf("Supplied report interval is %v", reportInterval)

	overrideReportIntervalMx.Lock()
	if overrideReportInterval != 0 {
		log.Debugf("Overriding report interval; new interval is %v", overrideReportInterval)
		reportInterval = overrideReportInterval
	}
	overrideReportIntervalMx.Unlock()

	if common.InDevelopment() {
		if reportInterval > 1*time.Minute {
			log.Debug("In development, will report everything to borda every 1 minutes")
			reportInterval = 1 * time.Minute
		}
		enabled = func(ctx map[string]interface{}) bool {
			return true
		}
	}

	log.Debugf("ReportInterval is %v", reportInterval)
	if reportInterval <= 0 {
		log.Debug("Borda not enabled")
		return
	}

	bordaURLMx.Lock()
	url := bordaURL
	bordaURLMx.Unlock()

	bordaClientMx.Lock()
	if bordaClient == nil {
		log.Debugf("Creating new borda client with report interval %v", reportInterval)
		bordaClient = createBordaClient(reportInterval, url)
	} else {
		log.Debugf("Updating report interval to %v", reportInterval)
		bordaClient.SetBatchInterval(reportInterval)
	}
	bordaClientMx.Unlock()
	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000)
	ConfigureWithSubmitter(reportToBorda, enabled)
}

// ConfigureWithSubmitter starts borda reporting using the given submitter function to report.
// to borda. The service never stops once enabled. The service will check enabled each time
// before it reports to borda, however.
func ConfigureWithSubmitter(submitter borda.Submitter, enabled EnabledFunc) {
	once.Do(func() {
		log.Debug("Enabling borda")

		pruningSubmitter := func(values map[string]borda.Val, dimensions map[string]interface{}) error {
			delete(dimensions, "beam") // beam is only useful within a single client session.
			// For some ops, we don't randomly sample, we include all of them
			_op := dimensions["op"]
			op, ok := _op.(string)
			if ok {
				for _, lightweightOp := range LightweightOps {
					if op == lightweightOp {
						log.Tracef("Removing high dimensional data for lightweight op %v", lightweightOp)
						delete(dimensions, "error")
						delete(dimensions, "error_text")
						delete(dimensions, "origin")
						delete(dimensions, "origin_host")
						delete(dimensions, "origin_port")
						delete(dimensions, "root_op")
					}
				}
			}

			sanitizeDimensions(dimensions)
			return submitter(values, dimensions)
		}
		startBorda(pruningSubmitter, enabled)
	})
}

// Flush flushes any pending submission
func Flush() {
	if bordaClient != nil {
		log.Debugf("Flushing pending borda submissions")
		bordaClientMx.Lock()
		bc := bordaClient
		bordaClientMx.Unlock()
		bc.Flush()
	}
}

func startBorda(reportToBorda borda.Submitter, enabled EnabledFunc) {
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

		err := reportToBorda(values, ctx)
		if err != nil {
			log.Debugf("failed to report to borda: %v", err)
		}
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

func createBordaClient(reportInterval time.Duration, url string) *borda.Client {
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
		URL:          url,
	})
}

// sanitizeDimensions sanitizes error strings in a context the same way that they're already sanitized in Borda using the SANITIZE alias
func sanitizeDimensions(dims map[string]interface{}) {
	for key, value := range dims {
		if key == "error" || key == "error_text" {
			errorString, ok := value.(string)
			if ok {
				dims[key] = sanitizeAddrsRegex.ReplaceAllString(errorString, "<addr>")
			}
		}
	}
}
