package otel

import (
	"context"
	"errors"
	"regexp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

// SetupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	return SetupOTelSDKWithConfig(ctx, DefaultConfig())
}

// Set up OpenTelemetry pipeline with a custom config.
func SetupOTelSDKWithConfig(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTracerProvider(ctx, cfg)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, cfg)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, cfg Config) (*trace.TracerProvider, error) {
	var exporter trace.SpanExporter
	var err error
	switch cfg.TracesExporter {
	case ExporterOTLP:
		var otlpTraceClient otlptrace.Client
		if cfg.isGRPC() {
			otlpTraceClient = otlptracegrpc.NewClient()
		} else {
			otlpTraceClient = otlptracehttp.NewClient()
		}
		exporter, err = otlptrace.New(
			ctx,
			otlpTraceClient,
		)
		if err != nil {
			return nil, err
		}
	case ExporterConsole:
		fallthrough
	default:
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, err
		}
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			// Default is 5s.
			trace.WithBatchTimeout(cfg.TracesBatchTimeout)),
		trace.WithSampler(newURLSampler(trace.AlwaysSample(), cfg.ExcludedURLPatterns)),
	)
	return tracerProvider, nil
}

func newMeterProvider(ctx context.Context, cfg Config) (*metric.MeterProvider, error) {
	var exporter metric.Exporter
	var err error
	switch cfg.MetricsExporter {
	case ExporterOTLP:
		if cfg.isGRPC() {
			exporter, err = otlpmetricgrpc.New(ctx)
		} else {
			exporter, err = otlpmetrichttp.New(ctx)
		}
		if err != nil {
			return nil, err
		}
	case ExporterConsole:
		fallthrough
	default:
		exporter, err = stdoutmetric.New()
		if err != nil {
			return nil, err
		}
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter,
			// Default is 1m.
			metric.WithInterval(cfg.MetricsInterval),
		)),
	)
	return meterProvider, nil
}

func newLoggerProvider() (*log.LoggerProvider, error) {
	logExporter, err := stdoutlog.New()
	if err != nil {
		return nil, err
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
	return loggerProvider, nil
}

// URLFilteringSampler implements a custom sampler to exclude specific URLs from tracing.
type URLFilteringSampler struct {
	delegate     trace.Sampler
	excludedURLs []*regexp.Regexp
}

func newURLSampler(delegate trace.Sampler, excludedPatterns []string) *URLFilteringSampler {
	var regexps []*regexp.Regexp
	for _, p := range excludedPatterns {
		regexps = append(regexps, regexp.MustCompile(p))
	}
	return &URLFilteringSampler{
		delegate:     delegate,
		excludedURLs: regexps,
	}
}

// ShouldSample implements the trace.Sampler interface.
func (s *URLFilteringSampler) ShouldSample(p trace.SamplingParameters) trace.SamplingResult {
	for _, attr := range p.Attributes {
		// Dont trace options requests
		if attr.Key == semconv.HTTPMethodKey && attr.Value.AsString() == "OPTIONS" {
			return trace.SamplingResult{Decision: trace.Drop}
		} else if attr.Key == semconv.HTTPURLKey {
			url := attr.Value.AsString()
			for _, r := range s.excludedURLs {
				if r.MatchString(url) {
					return trace.SamplingResult{Decision: trace.Drop}
				}
			}
		}
	}
	return s.delegate.ShouldSample(p)
}

// Description implements the trace.Sampler interface.
func (s *URLFilteringSampler) Description() string {
	return "URLFilteringSampler"
}
