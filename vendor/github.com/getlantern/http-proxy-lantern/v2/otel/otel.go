package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"

	"github.com/getlantern/golog"
)

const (
	batchTimeout = 1 * time.Minute
	maxQueueSize = 10000
)

var (
	log = golog.LoggerFor("otel")
)

type Opts struct {
	Endpoint      string
	Headers       map[string]string
	SampleRate    int
	ExternalIP    string
	ProxyName     string
	Track         string
	DC            string
	ProxyProtocol string
	IsPro         bool
}

func BuildTracerProvider(opts *Opts) (*sdktrace.TracerProvider, func()) {
	// Create HTTP client to talk to OTEL collector
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(opts.Endpoint),
		otlptracehttp.WithHeaders(opts.Headers),
	)

	// Create an exporter that exports to the OTEL collector
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		log.Errorf("Unable to initialize OpenTelemetry, will not report traces to %v", opts.Endpoint)
		return nil, func() {}
	}
	log.Debugf("Will report traces to OpenTelemetry at %v", opts.Endpoint)

	// Create a TracerProvider that uses the above exporter
	attributes := []attribute.KeyValue{
		semconv.ServiceNameKey.String("http-proxy-lantern"),
		attribute.String("protocol", opts.ProxyProtocol),
		attribute.Bool("pro", opts.IsPro),
	}
	if opts.Track != "" {
		attributes = append(attributes, attribute.String("track", opts.Track))
	}
	if opts.ExternalIP != "" {
		log.Debugf("Will report with external_ip: %v", opts.ExternalIP)
		attributes = append(attributes, attribute.String("external_ip", opts.ExternalIP))
	}
	if opts.ProxyName != "" {
		log.Debugf("Will report with proxy_name %v in dc %v", opts.ProxyName, opts.DC)
		attributes = append(attributes, attribute.String("proxy_name", opts.ProxyName))
		attributes = append(attributes, attribute.String("dc", opts.DC))
	}

	resource := resource.NewWithAttributes(semconv.SchemaURL, attributes...)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBatchTimeout(batchTimeout),
			sdktrace.WithMaxQueueSize(maxQueueSize),
			sdktrace.WithBlocking(), // it's okay to use blocking mode right now because we're just submitting bandwidth data in a goroutine that doesn't block real work
		),
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.ParentBased(newDeterministicSampler(opts.SampleRate))),
	)

	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Errorf("Error shutting down TracerProvider: %v", err)
		}
		if err := exporter.Shutdown(ctx); err != nil {
			log.Errorf("Error shutting down Exporter: %v", err)
		}
	}

	return tp, stop
}
