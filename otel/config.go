package otel

import (
	"os"
	"strings"
	"time"
)

type ExporterType string

const (
	ExporterOTLP    ExporterType = "otlp"
	ExporterConsole ExporterType = "console"
)

type Protocol string

const (
	ProtocolGRPC Protocol = "grpc"
	ProtocolHTTP Protocol = "http"
)

type Config struct {
	TracesExporter      ExporterType
	MetricsExporter     ExporterType
	Protocol            Protocol
	Endpoint            string
	ExcludedURLPatterns []string
	TracesBatchTimeout  time.Duration
	MetricsInterval     time.Duration
}

func DefaultConfig() Config {
	cfg := Config{
		ExcludedURLPatterns: []string{
			`^\/$`,
			`health$`,
			`^/metrics`,
		},
		TracesBatchTimeout: 5 * time.Second,
		MetricsInterval:    1 * time.Minute,
	}

	cfg.Protocol = Protocol(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"))
	cfg.Endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	cfg.TracesExporter = ExporterType(os.Getenv("OTEL_TRACES_EXPORTER"))
	cfg.MetricsExporter = ExporterType(os.Getenv("OTEL_METRICS_EXPORTER"))

	return cfg
}

func (c Config) isGRPC() bool {
	if c.Protocol == ProtocolGRPC {
		return true
	}
	if c.Protocol == ProtocolHTTP {
		return false
	}
	return strings.HasSuffix(strings.TrimSpace(c.Endpoint), "4317")
}
