package otel

import (
	"crypto/sha1"
	"math"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// This is taken from https://github.com/honeycombio/opentelemetry-samplers-go/blob/main/honeycombsamplers/deterministic_sampler.go
// I copied it because the published version has some dependency conflicts with OTEL.
type deterministicSampler struct {
	sampleRate int
	upperBound uint32
}

func newDeterministicSampler(sampleRate int) *deterministicSampler {
	// Get the actual upper bound - the largest possible value divided by
	// the sample rate. In the case where the sample rate is 1, this should
	// sample every value.
	upperBound := math.MaxUint32 / uint32(sampleRate)
	return &deterministicSampler{
		sampleRate: int(sampleRate),
		upperBound: upperBound,
	}
}

// bytesToUint32 takes a slice of 4 bytes representing a big endian 32 bit
// unsigned value and returns the equivalent uint32.
func bytesToUint32be(b []byte) uint32 {
	return uint32(b[3]) | (uint32(b[2]) << 8) | (uint32(b[1]) << 16) | (uint32(b[0]) << 24)
}

func (ds *deterministicSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	psc := trace.SpanContextFromContext(p.ParentContext)

	rate := uint32(ds.sampleRate)
	if rate < 1 {
		rate = 1
	}

	attrs := []attribute.KeyValue{
		attribute.Int("SampleRate", int(rate)),
	}

	if rate == 1 {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Attributes: attrs,
			Tracestate: psc.TraceState(),
		}
	}

	determinant := []byte(p.TraceID[:])
	sum := sha1.Sum([]byte(determinant))
	v := bytesToUint32be(sum[:4])

	upperBound := math.MaxUint32 / rate
	var decision sdktrace.SamplingDecision
	if v <= upperBound {
		decision = sdktrace.RecordAndSample
	} else {
		decision = sdktrace.Drop
	}

	return sdktrace.SamplingResult{
		Decision:   decision,
		Attributes: attrs,
		Tracestate: psc.TraceState(),
	}
}

func (ds *deterministicSampler) Description() string {
	return "HoneycombDeterministicSampler"
}
