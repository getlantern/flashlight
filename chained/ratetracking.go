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

func (p *proxy) withRateTracking(wrapped net.Conn, origin string) net.Conn {
	if wrapped == nil {
		return nil
	}

	return measured.Wrap(wrapped, rateInterval, func(conn measured.Conn) {
		stats := conn.Stats()
		rwError := conn.FirstError()
		if rwError == nil {
			p.consecRWSuccesses.Inc()
		} else {
			p.consecRWSuccesses.Dec()
		}
		// record simple traffic without origin
		op := ops.Begin("traffic").ChainedProxy(p.Name(), p.Addr(), p.Protocol(), p.Network())
		op.SetMetric("client_bytes_sent", borda.Sum(stats.SentTotal)).
			SetMetric("client_bytes_recv", borda.Sum(stats.RecvTotal))
		op.FailIf(rwError)
		p.onFinish(op)
		op.End()

		// record xfer data with origin
		op = ops.Begin("xfer").Origin(origin, "")
		op.SetMetric("client_bytes_sent", borda.Sum(stats.SentTotal)).
			SetMetric("client_bps_sent_min", borda.Min(stats.SentMin)).
			SetMetric("client_bps_sent_max", borda.Max(stats.SentMax)).
			SetMetric("client_bps_sent_avg", borda.WeightedAvg(stats.SentAvg, float64(stats.SentTotal))).
			SetMetric("client_bytes_recv", borda.Sum(stats.RecvTotal)).
			SetMetric("client_bps_recv_min", borda.Min(stats.RecvMin)).
			SetMetric("client_bps_recv_max", borda.Max(stats.RecvMax)).
			SetMetric("client_bps_recv_avg", borda.WeightedAvg(stats.RecvAvg, float64(stats.RecvTotal)))
		op.FailIf(rwError)
		p.onFinish(op)
		atomic.AddUint64(&p.dataSent, uint64(stats.SentTotal))
		atomic.AddUint64(&p.dataRecv, uint64(stats.RecvTotal))

		// The below is a little verbose, but it allows us to see the transfer rates
		// right within a user's logs, which is useful when someone submits their logs
		// together with a complaint of Lantern being slow.
		log.Infof("Finished xfer, received %v, total received %v",
			humanize.Bytes(uint64(stats.RecvTotal)),
			humanize.Bytes(atomic.AddUint64(&totalReceived, uint64(stats.RecvTotal))))
		op.End()
	})
}
