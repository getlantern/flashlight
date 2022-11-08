package bbr

import (
	"math"
	"sync"

	"github.com/getlantern/ema"
	"github.com/getlantern/mtime"
	"github.com/gonum/stat"
)

const (
	limit             = 25
	minSamples        = 5
	abeThreshold      = 2500000
	minBytesThreshold = 2048
	maxABE            = 100
)

type stats struct {
	sent    []float64
	abe     []float64
	weights []float64
	times   []mtime.Instant
	emaABE  *ema.EMA
	size    int
	idx     int
	mx      sync.RWMutex
}

func newStats() *stats {
	return &stats{
		sent:    make([]float64, limit),
		abe:     make([]float64, limit),
		weights: make([]float64, limit),
		times:   make([]mtime.Instant, limit),
		emaABE:  ema.New(0, 0.1),
	}
}

func (s *stats) update(sent float64, abe float64) {
	if sent < minBytesThreshold {
		// Don't bother recording values with too little data on connection
		return
	}

	now := mtime.Now()
	logOfSent := math.Log1p(sent)
	logOfABE := math.Log1p(abe)
	s.mx.Lock()
	s.sent[s.idx] = logOfSent
	s.abe[s.idx] = logOfABE
	s.weights[s.idx] = sent // give more weight to measurements from larger bytes sent
	s.times[s.idx] = now
	s.idx++
	if s.idx == limit {
		// Wrap
		s.idx = 0
	}
	if s.size < limit {
		s.size++
	}
	hasDelta := false
	for i := 1; i < s.size; i++ {
		if s.sent[i] != s.sent[i-1] {
			hasDelta = true
			break
		}
	}
	if !hasDelta {
		// There's no way to apply a regression, just ignore
		s.mx.Unlock()
		return
	}

	weights := make([]float64, 0, s.size)
	for i := 0; i < s.size; i++ {
		// Give more weight to more recent values
		age := now.Sub(s.times[i]).Seconds() + 1
		weights = append(weights, s.weights[i]/age)
	}

	// Estimate by applying a linear regression
	alpha, beta := stat.LinearRegression(s.sent[:s.size], s.abe[:s.size], weights, false)
	s.mx.Unlock()
	newEstimate := math.Expm1(alpha + beta*math.Log1p(abeThreshold))
	if math.IsNaN(newEstimate) {
		// ignore
		return
	}
	if newEstimate < 0 {
		newEstimate = 0
	}
	if newEstimate > maxABE {
		// Cap estimate to 100 Mbps
		newEstimate = maxABE
	}
	updated := s.emaABE.Update(newEstimate)
	if updated <= 0 {
		log.Tracef("Calculated negative EBA of %.2f?!, setting to small value instead", updated)
		// Set estimate to a small value to show that we have something
		s.emaABE.Set(0.01)
	}
}

func (s *stats) clear() {
	s.mx.Lock()
	s.emaABE.Clear()
	s.size = 0
	s.idx = 0
	s.mx.Unlock()
}

// estABE estimates the ABE at bytes_sent = 2.5 MB using a logarithmic
// regression on the most recent measurements. If upstreamABE is non-zero, this
// returns the lesser of the tracked ABE and the upstreamABE.
func (s *stats) estABE(upstreamABE float64) float64 {
	s.mx.RLock()
	enoughData := s.size >= minSamples
	s.mx.RUnlock()
	if !enoughData {
		return 0
	}
	downstreamABE := s.emaABE.Get()
	if upstreamABE > upstreamABEUnknown && upstreamABE < downstreamABE {
		return upstreamABE
	}
	return downstreamABE
}
