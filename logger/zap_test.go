package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// zapLogEntry represents a parsed JSON log line from the zap logger.
type zapLogEntry struct {
	Ts      float64 `json:"ts"`
	Level   string  `json:"level"`
	Message string  `json:"msg"`
	Logger  string  `json:"logger"`
}

func newTestZapLogger(level LogLevel) (*zapLogger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf, WithLevel(level))
	return logger, &buf
}

func parseZapLines(t *testing.T, buf *bytes.Buffer) []zapLogEntry {
	t.Helper()
	var entries []zapLogEntry
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry zapLogEntry
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "failed to parse JSON line: %s", line)
		entries = append(entries, entry)
	}
	return entries
}

func parseRawJSON(t *testing.T, buf *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var raw map[string]interface{}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 1)
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &raw))
	return raw
}

func levels(entries []zapLogEntry) []string {
	var result []string
	for _, e := range entries {
		result = append(result, e.Level)
	}
	return result
}

func TestZapLogger(t *testing.T) {
	tests := []struct {
		level            LogLevel
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			level:            LevelTrace,
			shouldContain:    []string{"trace", "debug", "info", "warn", "error"},
			shouldNotContain: []string{},
		},
		{
			level:            LevelDebug,
			shouldContain:    []string{"debug", "info", "warn", "error"},
			shouldNotContain: []string{"trace"},
		},
		{
			level:            LevelInfo,
			shouldContain:    []string{"info", "warn", "error"},
			shouldNotContain: []string{"trace", "debug"},
		},
		{
			level:            LevelWarn,
			shouldContain:    []string{"warn", "error"},
			shouldNotContain: []string{"trace", "debug", "info"},
		},
		{
			level:            LevelError,
			shouldContain:    []string{"error"},
			shouldNotContain: []string{"trace", "debug", "info", "warn"},
		},
	}

	for _, tt := range tests {
		logger, buf := newTestZapLogger(tt.level)
		logger.Trace("trace msg")
		logger.Debug("debug msg")
		logger.Info("info msg")
		logger.Warn("warn msg")
		logger.Error("error msg")
		_ = logger.sugar.Sync()

		entries := parseZapLines(t, buf)
		sev := levels(entries)
		for _, s := range tt.shouldContain {
			assert.Contains(t, sev, s, "level %v should contain %s", tt.level, s)
		}
		for _, s := range tt.shouldNotContain {
			assert.NotContains(t, sev, s, "level %v should not contain %s", tt.level, s)
		}
	}
}

func TestZapLoggerWithEnvLevel(t *testing.T) {
	tests := []struct {
		envLevel         string
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			envLevel:         "trace",
			shouldContain:    []string{"trace", "debug", "info", "warn", "error"},
			shouldNotContain: []string{},
		},
		{
			envLevel:         "debug",
			shouldContain:    []string{"debug", "info", "warn", "error"},
			shouldNotContain: []string{"trace"},
		},
		{
			envLevel:         "info",
			shouldContain:    []string{"info", "warn", "error"},
			shouldNotContain: []string{"trace", "debug"},
		},
		{
			envLevel:         "WARN",
			shouldContain:    []string{"warn", "error"},
			shouldNotContain: []string{"trace", "debug", "info"},
		},
		{
			envLevel:         "error",
			shouldContain:    []string{"error"},
			shouldNotContain: []string{"trace", "debug", "info", "warn"},
		},
	}

	for _, tt := range tests {
		os.Setenv("SM_LOG_LEVEL", tt.envLevel)
		var buf bytes.Buffer
		logger := newZapLoggerWithWriter(&buf)
		logger.Trace("trace msg")
		logger.Debug("debug msg")
		logger.Info("info msg")
		logger.Warn("warn msg")
		logger.Error("error msg")
		_ = logger.sugar.Sync()
		os.Unsetenv("SM_LOG_LEVEL")

		entries := parseZapLines(t, &buf)
		sev := levels(entries)
		for _, s := range tt.shouldContain {
			assert.Contains(t, sev, s, "env %s should contain %s", tt.envLevel, s)
		}
		for _, s := range tt.shouldNotContain {
			assert.NotContains(t, sev, s, "env %s should not contain %s", tt.envLevel, s)
		}
	}
}

func TestZapLoggerWith(t *testing.T) {
	logger, buf := newTestZapLogger(LevelInfo)
	derived := logger.With(map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	derived.Info("with metadata")
	_ = derived.(*zapLogger).sugar.Sync()

	raw := parseRawJSON(t, buf)
	assert.Equal(t, "with metadata", raw["msg"])
	// Fields are flat top-level, not nested under "metadata"
	assert.Equal(t, "value1", raw["key1"])
	assert.Equal(t, "value2", raw["key2"])
}

func TestZapLoggerWithAdditive(t *testing.T) {
	logger, buf := newTestZapLogger(LevelInfo)
	parent := logger.With(map[string]interface{}{"key1": "value1"})
	child := parent.With(map[string]interface{}{"key2": "value2"})
	child.Info("additive")
	_ = child.(*zapLogger).sugar.Sync()

	raw := parseRawJSON(t, buf)
	assert.Equal(t, "value1", raw["key1"])
	assert.Equal(t, "value2", raw["key2"])
}

func TestZapLoggerWithNil(t *testing.T) {
	logger, buf := newTestZapLogger(LevelInfo)
	// Must not panic, returns same logger
	derived := logger.With(nil)
	derived.Info("after nil with")
	_ = derived.(*zapLogger).sugar.Sync()

	entries := parseZapLines(t, buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "after nil with", entries[0].Message)
}

func TestZapLoggerWithPrefix(t *testing.T) {
	logger, buf := newTestZapLogger(LevelInfo)

	// WithPrefix uses zap's Named() — adds a "logger" field
	p1 := logger.WithPrefix("[myservice]")
	p1.Info("prefix test")
	_ = p1.(*zapLogger).sugar.Sync()

	raw := parseRawJSON(t, buf)
	assert.Equal(t, "[myservice]", raw["logger"])

	// Chained prefix appends with dot separator (zap Named behavior)
	buf.Reset()
	p2 := p1.WithPrefix("[subsystem]")
	p2.Info("chained prefix")
	_ = p2.(*zapLogger).sugar.Sync()

	raw = parseRawJSON(t, buf)
	assert.Equal(t, "[myservice].[subsystem]", raw["logger"])
}

func TestZapLoggerPrintfFormatting(t *testing.T) {
	logger, buf := newTestZapLogger(LevelTrace)
	logger.Info("hello %s %d", "world", 42)
	_ = logger.sugar.Sync()

	entries := parseZapLines(t, buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello world 42", entries[0].Message)
}

func TestZapLoggerTrace(t *testing.T) {
	// At Trace level, both Trace and Debug should appear with distinct severities
	logger, buf := newTestZapLogger(LevelTrace)
	logger.Trace("trace msg")
	logger.Debug("debug msg")
	_ = logger.sugar.Sync()

	entries := parseZapLines(t, buf)
	require.Len(t, entries, 2)
	assert.Equal(t, "trace", entries[0].Level)
	assert.Equal(t, "debug", entries[1].Level)

	// At Debug level, Trace should be suppressed
	logger2, buf2 := newTestZapLogger(LevelDebug)
	logger2.Trace("trace msg")
	logger2.Debug("debug msg")
	_ = logger2.sugar.Sync()

	entries2 := parseZapLines(t, buf2)
	require.Len(t, entries2, 1)
	assert.Equal(t, "debug", entries2[0].Level)
}

func TestZapLoggerLevelNone(t *testing.T) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf, WithLevel(LevelNone))
	logger.Trace("trace")
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")
	_ = logger.sugar.Sync()

	assert.Empty(t, buf.String(), "LevelNone should produce zero output")
}

func TestZapLoggerFieldNames(t *testing.T) {
	logger, buf := newTestZapLogger(LevelInfo)
	logger.Info("field names")
	_ = logger.sugar.Sync()

	raw := parseRawJSON(t, buf)
	assert.Contains(t, raw, "ts")
	assert.Contains(t, raw, "level")
	assert.Contains(t, raw, "msg")
}

func TestZapLoggerWarnLevel(t *testing.T) {
	logger, buf := newTestZapLogger(LevelWarn)
	logger.Warn("warning test")
	_ = logger.sugar.Sync()

	entries := parseZapLines(t, buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "warn", entries[0].Level)
}

func TestZapLoggerFlush(t *testing.T) {
	logger, _ := newTestZapLogger(LevelInfo)
	err := logger.Flush()
	assert.NoError(t, err)
}

func TestZapLoggerWithContextNil(t *testing.T) {
	logger, _ := newTestZapLogger(LevelInfo)
	result := logger.WithContext(nil)
	assert.Equal(t, logger, result, "WithContext(nil) should return self")
}

func TestZapLoggerWithContextEmpty(t *testing.T) {
	logger, _ := newTestZapLogger(LevelInfo)
	result := logger.WithContext(context.Background())
	assert.Equal(t, logger, result, "WithContext with empty context should return self")
}

func TestZapLoggerWithContextNoCorrelation(t *testing.T) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf, WithLevel(LevelInfo))

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	result := logger.WithContext(ctx)
	assert.Equal(t, logger, result, "without GCP correlation, WithContext should return self")
}

func TestZapLoggerWithContextOTEL(t *testing.T) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf, WithLevel(LevelInfo), WithGCPTraceCorrelation())

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	derived := logger.WithContext(ctx)
	derived.Info("otel test")
	_ = derived.(*zapLogger).sugar.Sync()

	raw := parseRawJSON(t, &buf)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", raw["logging.googleapis.com/trace"])
	assert.Equal(t, "0102030405060708", raw["logging.googleapis.com/spanId"])
	assert.Equal(t, true, raw["logging.googleapis.com/trace_sampled"])
}

func TestZapLoggerWithLevel(t *testing.T) {
	logger, buf := newTestZapLogger(LevelWarn)
	logger.Info("should not appear")
	logger.Warn("should appear")
	_ = logger.sugar.Sync()

	entries := parseZapLines(t, buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "warn", entries[0].Level)
}

func TestZapLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf, WithLevel(LevelInfo), WithFields(map[string]interface{}{
		"service": "test-svc",
		"version": "1.0",
	}))
	logger.Info("fields test")
	_ = logger.sugar.Sync()

	raw := parseRawJSON(t, &buf)
	assert.Equal(t, "test-svc", raw["service"])
	assert.Equal(t, "1.0", raw["version"])
}

func TestZapLoggerWithGCPTraceCorrelation(t *testing.T) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf, WithLevel(LevelInfo), WithGCPTraceCorrelation())
	assert.True(t, logger.gcpTraceCorrelation, "option should set gcpTraceCorrelation flag")
}

func TestZapLoggerCombinedOptions(t *testing.T) {
	var buf bytes.Buffer
	logger := newZapLoggerWithWriter(&buf,
		WithLevel(LevelDebug),
		WithFields(map[string]interface{}{"env": "test"}),
		WithGCPTraceCorrelation(),
	)

	assert.True(t, logger.gcpTraceCorrelation)

	logger.Debug("combined test")
	_ = logger.sugar.Sync()

	raw := parseRawJSON(t, &buf)
	assert.Equal(t, "test", raw["env"])
	assert.Equal(t, "debug", raw["level"])
}

func TestMuxLoggerFlush(t *testing.T) {
	l1, _ := newTestZapLogger(LevelInfo)
	l2, _ := newTestZapLogger(LevelInfo)
	mux := NewMultiLogger(l1, l2)

	err := mux.Flush()
	assert.NoError(t, err)
}

func TestMuxLoggerWithContext(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	l1 := newZapLoggerWithWriter(&buf1, WithLevel(LevelInfo), WithGCPTraceCorrelation())
	l2 := newZapLoggerWithWriter(&buf2, WithLevel(LevelInfo), WithGCPTraceCorrelation())
	mux := NewMultiLogger(l1, l2)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	derived := mux.WithContext(ctx)
	derived.Info("mux context test")

	for i, buf := range []*bytes.Buffer{&buf1, &buf2} {
		entries := parseZapLines(t, buf)
		require.Len(t, entries, 1, "logger %d should have 1 entry", i)
		assert.Equal(t, "mux context test", entries[0].Message)
	}
}
