package borda

import (
	"math/rand"
	"time"

	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/borda/client/rpcclient"
	"github.com/getlantern/context"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/http-proxy/listeners"
	"github.com/getlantern/measured"
	"github.com/getlantern/ops"
)

const (
	nanosPerSecond = 1000 * 1000 * 1000
)

var (
	log = golog.LoggerFor("lantern-proxy-borda")

	fullyReportedOps = []string{"http_proxy_handle", "tcpinfo", "google_search", "google_captcha", "blacklist", "connect_without_request", "mimic_apache"}
)

// Enable enables borda reporting
func Enable(bordaReportInterval time.Duration, bordaSamplePercentage float64, maxBufferSize int) listeners.MeasuredReportFN {
	inSample := func(ctx map[string]interface{}) bool {
		if rand.Float64() < bordaSamplePercentage {
			return true
		}

		// For some ops, we don't randomly sample, we include all of them
		op := ctx["op"]
		switch t := op.(type) {
		case string:
			for _, fullyReportedOp := range fullyReportedOps {
				if t == fullyReportedOp {
					log.Tracef("Including fully reported op %v in borda sample", fullyReportedOp)
					return true
				}
			}
			return false
		default:
			return false
		}
	}

	bordaClient := rpcclient.Default(bordaReportInterval)
	reportToBorda := bordaClient.ReducingSubmitter("proxy_results", maxBufferSize)

	ops.RegisterReporter(func(failure error, ctx map[string]interface{}) {
		if !inSample(ctx) {
			return
		}

		values := map[string]borda.Val{}
		if failure != nil {
			values["error_count"] = borda.Float(1)
		} else {
			values["success_count"] = borda.Float(1)
		}

		reportToBorda(values, ctx)
	})

	golog.RegisterReporter(func(err error, severity golog.Severity, ctx map[string]interface{}) {
		if !inSample(ctx) {
			return
		}

		values := map[string]borda.Val{
			"error_count": borda.Float(1),
		}
		ctx["error"] = hidden.Clean(err.Error())
		reportToBorda(values, ctx)
	})

	return func(_ctx map[string]interface{}, stats *measured.Stats, deltaStats *measured.Stats, final bool) {
		if !final {
			// We report only the final values
			return
		}

		vals := map[string]borda.Val{
			"server_bytes_sent":          borda.Float(stats.SentTotal),
			"server_bps_sent_min":        borda.Min(stats.SentMin),
			"server_bps_sent_max":        borda.Max(stats.SentMax),
			"server_bps_sent_avg":        borda.WeightedAvg(stats.SentAvg, float64(stats.SentTotal)),
			"server_bytes_recv":          borda.Float(stats.RecvTotal),
			"server_bps_recv_min":        borda.Min(stats.RecvMin),
			"server_bps_recv_max":        borda.Max(stats.RecvMax),
			"server_bps_recv_avg":        borda.WeightedAvg(stats.RecvAvg, float64(stats.RecvTotal)),
			"server_connection":          borda.Float(1),
			"server_connection_duration": borda.Float(float64(stats.Duration) / nanosPerSecond),
		}
		log.Tracef("xfer: %v %v", _ctx, vals)

		if !inSample(_ctx) {
			return
		}

		// Copy the context before modifying it
		ctx := make(context.Map, len(_ctx))
		for key, value := range _ctx {
			ctx[key] = value
		}
		ctx["op"] = "xfer"

		reportErr := reportToBorda(vals, ops.AsMap(ctx, true))
		if reportErr != nil {
			log.Errorf("Error reporting error to borda: %v", reportErr)
		}
	}
}

// ConnectionTypedBordaReporter adds a conn_type dimension to measured stats
// reported to Borda.
func ConnectionTypedBordaReporter(connType string, bordaReporter listeners.MeasuredReportFN) listeners.MeasuredReportFN {
	return func(ctx map[string]interface{}, stats *measured.Stats, deltaStats *measured.Stats, final bool) {
		ctx["conn_type"] = connType
		bordaReporter(ctx, stats, deltaStats, final)
	}
}
