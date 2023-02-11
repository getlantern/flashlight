package quicwrapper

import (
	"math"
	"sync"
	"time"

	"github.com/getlantern/ema"
	"github.com/getlantern/ops"
	quic "github.com/lucas-clemente/quic-go"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	bandwidthHistogram = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "quic_bandwidth_hist",
			Help:       "Bandwidth estimate histogram in bytes per second",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{},
	)
)

func init() {
	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(bandwidthHistogram)
}

type Bandwidth = quic.Bandwidth

const (
	Mib = 1024 * 1024 // 1 Mebibit Bandwidth
)

type BandwidthEstimator interface {
	BandwidthEstimate() Bandwidth
}

const (
	EMABandwidthSamplerDefaultPeriod = 1 * time.Second
	EMABandwidthSamplerDefaultWindow = 15 * time.Second
)

// Samples and averages bandwidth estimates from another
// BandwidthEstimator
type EMABandwidthSampler struct {
	estimate *ema.EMA
	source   BandwidthEstimator
	period   time.Duration
	window   time.Duration
	done     chan struct{}
	start    sync.Once
	stop     sync.Once
}

func NewEMABandwidthSampler(from BandwidthEstimator) *EMABandwidthSampler {
	return NewEMABandwidthSamplerParams(from, EMABandwidthSamplerDefaultPeriod, EMABandwidthSamplerDefaultWindow)
}

func NewEMABandwidthSamplerParams(from BandwidthEstimator, period time.Duration, window time.Duration) *EMABandwidthSampler {

	alpha := 1.0 - math.Exp(-float64(period)/float64(window))

	return &EMABandwidthSampler{
		estimate: ema.New(0, alpha),
		source:   from,
		period:   period,
		done:     make(chan struct{}),
	}
}

func (bs *EMABandwidthSampler) BandwidthEstimate() Bandwidth {
	return Bandwidth(bs.estimate.Get())
}

func (bs *EMABandwidthSampler) Start() {
	bs.start.Do(func() {
		ops.Go(func() {
			for {
				select {
				case <-bs.done:
					return
				case <-time.After(bs.period):
					bs.update()
				}
			}
		})
	})
}

func (bs *EMABandwidthSampler) Stop() {
	bs.stop.Do(func() {
		close(bs.done)
	})
}

func (bs *EMABandwidthSampler) Clear() {
	bs.estimate.Clear()
}

func (bs *EMABandwidthSampler) update() {
	sample := bs.source.BandwidthEstimate()
	// It's worth knowing that sample here does not mean that the proxy is doing that much
	// just that the congestion control algro thinks at this moment in time it could go that
	// fast. Histograms are useful here since proxy connections are not always busy, and so
	// will give invalid-ish (for most human cases) BandwidthEstimate readings.
	bandwidthHistogram.WithLabelValues().Observe(float64(sample))
	bs.estimate.Update(float64(sample))
}
