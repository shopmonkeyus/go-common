package logger

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const zapTraceLevel = zapcore.DebugLevel - 1

var zapLevels = map[LogLevel]zapcore.Level{
	LevelTrace: zapTraceLevel,
	LevelDebug: zapcore.DebugLevel,
	LevelInfo:  zapcore.InfoLevel,
	LevelWarn:  zapcore.WarnLevel,
	LevelError: zapcore.ErrorLevel,
	LevelPanic: zapcore.PanicLevel,
	LevelFatal: zapcore.FatalLevel,
}

type ZapOption func(*zapConfig)

type zapConfig struct {
	level               LogLevel
	levelSet            bool
	fields              map[string]interface{}
	gcpTraceCorrelation bool
}

type zapLogger struct {
	sugar               *zap.SugaredLogger
	gcpTraceCorrelation bool
}

var _ Logger = (*zapLogger)(nil)

func WithLevel(level LogLevel) ZapOption {
	return func(c *zapConfig) {
		c.level = level
		c.levelSet = true
	}
}

func WithFields(fields map[string]interface{}) ZapOption {
	return func(c *zapConfig) {
		c.fields = fields
	}
}

func WithGCPTraceCorrelation(enabled bool) ZapOption {
	return func(c *zapConfig) {
		c.gcpTraceCorrelation = enabled
	}
}

func NewZapLogger(opts ...ZapOption) Logger {
	cfg := applyZapOptions(opts)
	level := getLogLevel(cfg)
	if level == LevelNone {
		return nopZapLogger(cfg)
	}
	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(mapLogLevelToZap(level))
	zapCfg.EncoderConfig.EncodeLevel = levelEncoder
	base, _ := zapCfg.Build(zap.WithFatalHook(zapcore.WriteThenFatal))
	return buildZapLogger(base, cfg)
}

func (z *zapLogger) Trace(msg string, args ...interface{}) {
	z.sugar.Logf(zapTraceLevel, msg, args...)
}

func (z *zapLogger) Debug(msg string, args ...interface{}) {
	z.sugar.Debugf(msg, args...)
}

func (z *zapLogger) Info(msg string, args ...interface{}) {
	z.sugar.Infof(msg, args...)
}

func (z *zapLogger) Warn(msg string, args ...interface{}) {
	z.sugar.Warnf(msg, args...)
}

func (z *zapLogger) Error(msg string, args ...interface{}) {
	z.sugar.Errorf(msg, args...)
}

func (z *zapLogger) Fatal(msg string, args ...interface{}) {
	z.sugar.Fatalf(msg, args...)
}

func (z *zapLogger) WithFields(args ...interface{}) Logger {
	if len(args) == 0 {
		return z
	}
	return &zapLogger{
		sugar:               z.sugar.With(args...),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}

func (z *zapLogger) With(metadata map[string]interface{}) Logger {
	if len(metadata) == 0 {
		return z
	}
	fields := make([]interface{}, 0, len(metadata)*2)
	for k, v := range metadata {
		fields = append(fields, k, v)
	}
	return &zapLogger{
		sugar:               z.sugar.With(fields...),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}

func (z *zapLogger) WithPrefix(prefix string) Logger {
	return &zapLogger{
		sugar:               z.sugar.Named(prefix),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}

func (z *zapLogger) WithContext(ctx context.Context) Logger {
	if ctx == nil || !z.gcpTraceCorrelation {
		return z
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return z
	}
	return &zapLogger{
		sugar: z.sugar.With(
			zap.String("logging.googleapis.com/trace", sc.TraceID().String()),
			zap.String("logging.googleapis.com/spanId", sc.SpanID().String()),
			zap.Bool("logging.googleapis.com/trace_sampled", sc.IsSampled()),
		),
		gcpTraceCorrelation: z.gcpTraceCorrelation,
	}
}

func (z *zapLogger) Flush() error {
	err := z.sugar.Sync()
	if err == nil {
		return nil
	}
	// Ignore harmless PathError from syncing stdout/stderr — these can't be
	// fsync'd on most OS/terminal combinations (known Zap issue on macOS/Linux).
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return nil
	}
	return err
}

func applyZapOptions(opts []ZapOption) *zapConfig {
	cfg := &zapConfig{gcpTraceCorrelation: true}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func getLogLevel(cfg *zapConfig) LogLevel {
	if cfg.levelSet {
		return cfg.level
	}
	if _, ok := os.LookupEnv("SM_LOG_LEVEL"); ok {
		return GetLevelFromEnv()
	}
	return LevelInfo
}

func mapLogLevelToZap(level LogLevel) zapcore.Level {
	if zl, ok := zapLevels[level]; ok {
		return zl
	}
	return zapcore.InfoLevel
}

func levelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	if l == zapTraceLevel {
		enc.AppendString("trace")
		return
	}
	zapcore.LowercaseLevelEncoder(l, enc)
}

func buildZapLogger(base *zap.Logger, cfg *zapConfig) *zapLogger {
	if len(cfg.fields) > 0 {
		fields := make([]zap.Field, 0, len(cfg.fields))
		for k, v := range cfg.fields {
			fields = append(fields, zap.Any(k, v))
		}
		base = base.With(fields...)
	}
	return &zapLogger{
		sugar:               base.Sugar(),
		gcpTraceCorrelation: cfg.gcpTraceCorrelation,
	}
}

func nopZapLogger(cfg *zapConfig) *zapLogger {
	return &zapLogger{
		sugar:               zap.NewNop().Sugar(),
		gcpTraceCorrelation: cfg.gcpTraceCorrelation,
	}
}

func NewZapTestLogger(w io.Writer, opts ...ZapOption) *zapLogger {
	cfg := applyZapOptions(opts)
	logLevel := getLogLevel(cfg)
	if logLevel == LevelNone {
		return nopZapLogger(cfg)
	}
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeLevel = levelEncoder
	ws := zapcore.AddSync(w)
	encoder := zapcore.NewJSONEncoder(encCfg)
	core := zapcore.NewCore(encoder, ws, mapLogLevelToZap(logLevel))
	base := zap.New(core, zap.WithFatalHook(zapcore.WriteThenNoop))
	return buildZapLogger(base, cfg)
}
