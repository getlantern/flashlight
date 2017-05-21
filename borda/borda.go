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
	"github.com/getlantern/flashlight/service"
)

var (
	log = golog.LoggerFor("flashlight.borda")

	ServiceType service.Type = "flashlight.borda"

	// BeforeSubmit is an optional callback to capture when borda batches are
	// submitted. It's mostly useful for unit testing.
	BeforeSubmit func(name string, key string, ts time.Time, values map[string]borda.Val, dimensions map[string]interface{})
)

type ConfigOpts struct {
	ReportInterval time.Duration
	ReportAllOps   bool
}

func (c *ConfigOpts) For() service.Type {
	return ServiceType
}

func (c *ConfigOpts) Complete() string {
	if c.ReportInterval == 0 {
		return "ReportInterval is zero"
	}
	return ""
}

type bordaService struct {
	muOpts           sync.RWMutex
	opts             ConfigOpts
	fullyReportedOps []string
	bordaClient      *borda.Client
}

func New(fullyReportedOps []string) service.Impl {
	return &bordaService{fullyReportedOps: fullyReportedOps}
}

func (s *bordaService) GetType() service.Type {
	return ServiceType
}

func (s *bordaService) Configure(opts service.ConfigOpts) {
	o := opts.(*ConfigOpts)
	shouldRestart := false
	s.muOpts.Lock()
	if s.opts.ReportInterval > 0 && s.opts.ReportInterval != o.ReportInterval {
		shouldRestart = true
	}
	s.opts = *o
	s.muOpts.Unlock()
	if shouldRestart {
		s.Stop()
		s.Start()
	}
}

func (s *bordaService) Start() {
	s.startBordaAndProxyBench()
}

func (s *bordaService) Stop() {
	if s.bordaClient != nil {
		log.Debugf("Flushing pending borda submissions")
		s.bordaClient.Flush()
	}
}

func (s *bordaService) startBordaAndProxyBench() {
	s.bordaClient = createBordaClient(s.opts.ReportInterval)

	reportToBorda := s.bordaClient.ReducingSubmitter("client_results", 1000)

	proxybench.Start(&proxybench.Opts{}, func(timing time.Duration, ctx map[string]interface{}) {
		// No need to do anything, this is now handled with the regular op reporting
	})

	reporter := func(failure error, ctx map[string]interface{}) {
		if !s.shouldReport(ctx) {
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
	log.Debug("Registered borda reporter")
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
				s.bordaClient.Flush()
			}
		}
	})
}

func (s *bordaService) shouldReport(ctx map[string]interface{}) bool {
	s.muOpts.RLock()
	should := s.opts.ReportAllOps
	s.muOpts.RUnlock()
	if should {
		return true
	}
	// For some ops, we don't randomly sample, we include all of them
	switch t := ctx["op"].(type) {
	case string:
		for _, op := range s.fullyReportedOps {
			if t == op {
				log.Tracef("Including fully reported op %v in borda sample", op)
				return true
			}
		}
		return false
	default:
		return false
	}
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
