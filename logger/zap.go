package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var zapLevels = map[LogLevel]zapcore.Level{
	LevelTrace: zapcore.DebugLevel,
	LevelDebug: zapcore.DebugLevel,
	LevelInfo:  zapcore.InfoLevel,
	LevelWarn:  zapcore.WarnLevel,
	LevelError: zapcore.ErrorLevel,
	LevelNone:  zapcore.FatalLevel,
}

func toZapLevel(level LogLevel) zap.AtomicLevel {
	if zl, ok := zapLevels[level]; ok {
		return zap.NewAtomicLevelAt(zl)
	}
	return zap.NewAtomicLevelAt(zapcore.InfoLevel)
}

// ZapOption configures the Zap logger.
type ZapOption func(*zapConfig)

type zapConfig struct {
	level               LogLevel
	fields              map[string]interface{}
	gcpTraceCorrelation bool
}

// WithLevel sets the minimum log level. Default is read from SM_LOG_LEVEL env var.
func WithLevel(level LogLevel) ZapOption {
	return func(c *zapConfig) { c.level = level }
}

// WithFields adds base fields to every log entry (e.g. service name, commit, pod).
func WithFields(fields map[string]interface{}) ZapOption {
	return func(c *zapConfig) { c.fields = fields }
}

// WithGCPTraceCorrelation enables automatic injection of OTEL span/trace IDs
// into log entries when retrieved via FromContext().
func WithGCPTraceCorrelation() ZapOption {
	return func(c *zapConfig) { c.gcpTraceCorrelation = true }
}

// zapLogger implements Logger backed by zap.SugaredLogger.
type zapLogger struct {
	log                 *zap.SugaredLogger
	gcpTraceCorrelation bool
}

var _ Logger = (*zapLogger)(nil)

// NewZapLogger creates a Logger backed by zap with production defaults.
func NewZapLogger(opts ...ZapOption) Logger {
	cfg := &zapConfig{level: GetLevelFromEnv()}
	for _, opt := range opts {
		opt(cfg)
	}

	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = toZapLevel(cfg.level)

	if len(cfg.fields) > 0 {
		zapCfg.InitialFields = cfg.fields
	}

	l, err := zapCfg.Build()
	if err != nil {
		panic(err)
	}

	return &zapLogger{
		log:                 l.Sugar(),
		gcpTraceCorrelation: cfg.gcpTraceCorrelation,
	}
}

func (z *zapLogger) With(metadata map[string]interface{}) Logger {
	fields := make([]interface{}, 0, len(metadata)*2)
	for k, v := range metadata {
		fields = append(fields, k, v)
	}
	return &zapLogger{
		log:                 z.log.With(fields...),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}

func (z *zapLogger) WithPrefix(prefix string) Logger {
	return &zapLogger{
		log:                 z.log.Named(prefix),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}

func (z *zapLogger) Trace(msg string, args ...interface{}) { z.log.Debugf(msg, args...) }
func (z *zapLogger) Debug(msg string, args ...interface{}) { z.log.Debugf(msg, args...) }
func (z *zapLogger) Info(msg string, args ...interface{})  { z.log.Infof(msg, args...) }
func (z *zapLogger) Warn(msg string, args ...interface{})  { z.log.Warnf(msg, args...) }
func (z *zapLogger) Error(msg string, args ...interface{}) { z.log.Errorf(msg, args...) }
func (z *zapLogger) Fatal(msg string, args ...interface{}) { z.log.Fatalf(msg, args...) }

func (z *zapLogger) Flush() error { return z.log.Sync() }

// WithContext returns a new logger enriched with OTEL trace/span IDs from the context.
func (z *zapLogger) WithContext(ctx context.Context) Logger {
	if !z.gcpTraceCorrelation {
		return z
	}
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		return z
	}
	return &zapLogger{
		log: z.log.With(
			"logging.googleapis.com/spanId", spanCtx.SpanID().String(),
			"logging.googleapis.com/trace", spanCtx.TraceID().String(),
			"logging.googleapis.com/trace_sampled", spanCtx.IsSampled(),
		),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}
