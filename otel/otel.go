package otel

import (
	"context"
	"crypto/sha1"
	"math"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
)

var (
	log = golog.LoggerFor("otel")

	stopper   func()
	stopperMx sync.Mutex
)

type Config struct {
	Endpoint      string
	Headers       map[string]string
	SampleRate    uint32
	OpSampleRates map[string]uint32
}

func Configure(cfg *Config) {
	log.Debugf("Configuring OpenTelemetry with sample rate %d and op sample rates %v", cfg.SampleRate, cfg.OpSampleRates)
	log.Debugf("Connecting to endpoint %v", cfg.Endpoint)
	log.Debugf("Using headers %v", cfg.Headers)

	if cfg.SampleRate < 1 {
		cfg.SampleRate = 1
	}
	if cfg.OpSampleRates == nil {
		cfg.OpSampleRates = make(map[string]uint32)
	}

	// Create HTTP client to talk to OTEL collector
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithHeaders(cfg.Headers),
	)

	// Create an exporter that exports to the OTEL collector
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		log.Errorf("Unable to initialize OpenTelemetry, will not report traces")
	} else {
		log.Debug("Will report traces to OpenTelemetry")
		// Create a TracerProvider that uses the above exporter
		resource :=
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String("flashlight"),
			)
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(resource),
			sdktrace.WithSampler(sdktrace.ParentBased(&opAwareSampler{
				sampleRate:    cfg.SampleRate,
				opSampleRates: cfg.OpSampleRates,
			})),
		)

		// Configure OTEL tracing to use the above TracerProvider
		otel.SetTracerProvider(tp)
		ops.EnableOpenTelemetry("flashlight")

		stopperMx.Lock()
		if stopper != nil {
			// this means that we reconfigured after previously setting up a TracerProvider, shut the old one down
			go stopper()
		}
		stopper = func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				log.Errorf("Error shutting down TracerProvider: %v", err)
			}
			if err := exporter.Shutdown(ctx); err != nil {
				log.Errorf("Error shutting down Exporter: %v", err)
			}
		}
		stopperMx.Unlock()
	}
}

func Stop() {
	stopperMx.Lock()
	defer stopperMx.Unlock()
	if stopper != nil {
		log.Debug("Stopping OpenTelemetry trace exporter")
		stopper()
		log.Debug("Stopped OpenTelemetry trace exporter")
	}
}

// opAwareSampler is a variation of Honeycomb's deterministic sampler that takes into account
// op-specific sample rates.
// See https://github.com/honeycombio/opentelemetry-samplers-go/blob/main/honeycombsamplers/deterministic_sampler.go.
type opAwareSampler struct {
	sampleRate    uint32
	opSampleRates map[string]uint32
}

func (s *opAwareSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	psc := trace.SpanContextFromContext(p.ParentContext)

	rate, found := s.opSampleRates[p.Name]
	if !found {
		rate = s.sampleRate
	}

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

func (s *opAwareSampler) Description() string {
	return "OpAwareSampler"
}

// bytesToUint32 takes a slice of 4 bytes representing a big endian 32 bit
// unsigned value and returns the equivalent uint32.
func bytesToUint32be(b []byte) uint32 {
	return uint32(b[3]) | (uint32(b[2]) << 8) | (uint32(b[1]) << 16) | (uint32(b[0]) << 24)
}
