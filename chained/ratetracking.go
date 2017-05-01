package chained

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	borda "github.com/getlantern/borda/client"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/measured"
)

const (
	rateInterval = 1 * time.Second
)

var totalReceived = uint64(0)

func withRateTracking(wrapped net.Conn, origin string, onFinish func(op *ops.Op)) net.Conn {
	return measured.Wrap(wrapped, rateInterval, func(conn measured.Conn) {
		stats := conn.Stats()
		// record simple traffic without origin
		op := ops.Begin("traffic")
		op.SetMetric("client_bytes_sent", borda.Float(stats.SentTotal)).
			SetMetric("client_bytes_recv", borda.Float(stats.RecvTotal))
		op.FailIf(conn.FirstError())
		op.End()

		// record xfer data with origin
		op = ops.Begin("xfer").Origin(origin, "")
		op.SetMetric("client_bytes_sent", borda.Float(stats.SentTotal)).
			SetMetric("client_bps_sent_min", borda.Min(stats.SentMin)).
			SetMetric("client_bps_sent_max", borda.Max(stats.SentMax)).
			SetMetric("client_bps_sent_avg", borda.WeightedAvg(stats.SentAvg, float64(stats.SentTotal))).
			SetMetric("client_bytes_recv", borda.Float(stats.RecvTotal)).
			SetMetric("client_bps_recv_min", borda.Min(stats.RecvMin)).
			SetMetric("client_bps_recv_max", borda.Max(stats.RecvMax)).
			SetMetric("client_bps_recv_avg", borda.WeightedAvg(stats.RecvAvg, float64(stats.RecvTotal)))
		op.FailIf(conn.FirstError())
		// The below is a little verbose, but it allows us to see the transfer rates
		// right within a user's logs, which is useful when someone submits their logs
		// together with a complaint of Lantern being slow.
		log.Debug("Finished xfer")
		log.Debugf("Total Received: %v", humanize.Bytes(atomic.AddUint64(&totalReceived, uint64(stats.RecvTotal))))
		op.End()
	})
}

func setDetails(conn measured.Conn, stats *measured.Stats, onFinish func(op *ops.Op), op *ops.Op) {
	op.SetMetric("client_bytes_sent", borda.Float(stats.SentTotal)).
		SetMetric("client_bps_sent_min", borda.Min(stats.SentMin)).
		SetMetric("client_bps_sent_max", borda.Max(stats.SentMax)).
		SetMetric("client_bps_sent_avg", borda.WeightedAvg(stats.SentAvg, float64(stats.SentTotal))).
		SetMetric("client_bytes_recv", borda.Float(stats.RecvTotal)).
		SetMetric("client_bps_recv_min", borda.Min(stats.RecvMin)).
		SetMetric("client_bps_recv_max", borda.Max(stats.RecvMax)).
		SetMetric("client_bps_recv_avg", borda.WeightedAvg(stats.RecvAvg, float64(stats.RecvTotal)))

	if onFinish != nil {
		onFinish(op)
	}
	op.FailIf(conn.FirstError())
}
