package logger

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func captureOutput(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	f()
	log.SetOutput(nil)
	return buf.String()
}

func TestConsoleLogger(t *testing.T) {

	logger := NewConsoleLogger().(*consoleLogger)

	tests := []struct {
		level            LogLevel
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			level:            LevelTrace,
			shouldContain:    []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"},
			shouldNotContain: []string{},
		},
		{
			level:            LevelDebug,
			shouldContain:    []string{"DEBUG", "INFO", "WARN", "ERROR"},
			shouldNotContain: []string{"TRACE"},
		},
		{
			level:            LevelInfo,
			shouldContain:    []string{"INFO", "WARN", "ERROR"},
			shouldNotContain: []string{"TRACE", "DEBUG"},
		},
		{
			level:            LevelWarn,
			shouldContain:    []string{"WARN", "ERROR"},
			shouldNotContain: []string{"TRACE", "DEBUG", "INFO"},
		},
		{
			level:            LevelError,
			shouldContain:    []string{"ERROR"},
			shouldNotContain: []string{"TRACE", "DEBUG", "INFO", "WARN"},
		},
	}

	for _, tt := range tests {
		logger.SetLogLevel(tt.level)
		output := captureOutput(func() {
			logger.Trace("This is a trace message")
			logger.Debug("This is a debug message")
			logger.Info("This is an info message")
			logger.Warn("This is a warn message")
			logger.Error("This is an error message")
		})
		for _, shouldContain := range tt.shouldContain {
			assert.Contains(t, output, shouldContain)
		}
		for _, shouldNotContain := range tt.shouldNotContain {
			assert.NotContains(t, output, shouldNotContain)
		}
	}
}

func TestConsoleLoggerWithMetadata(t *testing.T) {
	logger := NewConsoleLogger().(*consoleLogger)

	metadata := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	}
	logger = logger.With(metadata).(*consoleLogger)

	output := captureOutput(func() {
		logger.Info("This is an info message with metadata")
	})

	assert.Contains(t, output, "This is an info message with metadata")
	assert.Contains(t, output, `"key1":"value1"`)
	assert.Contains(t, output, `"key2":"value2"`)
}

func TestConsoleLoggerSinkTraceLevel(t *testing.T) {
	sink := &testSink{}
	logger := NewConsoleLogger().(*consoleLogger).WithSink(sink).(*consoleLogger)
	logger.SetLogLevel(LevelTrace)

	logger.Trace("This is a trace message")
	assert.Contains(t, string(sink.buf), "This is a trace message")

	logger.Debug("This is a debug message")
	assert.Contains(t, string(sink.buf), "This is a debug message")

	logger.Info("This is an info message")
	assert.Contains(t, string(sink.buf), "This is an info message")

	logger.Warn("This is a warn message")
	assert.Contains(t, string(sink.buf), "This is a warn message")

	logger.Error("This is an error message")
	assert.Contains(t, string(sink.buf), "This is an error message")
}

func TestConsoleLoggerSinkDebugLevel(t *testing.T) {
	sink := &testSink{}
	logger := NewConsoleLogger().(*consoleLogger).WithSink(sink).(*consoleLogger)
	logger.SetLogLevel(LevelDebug)

	logger.Trace("This trace message should not be printed")
	assert.NotContains(t, string(sink.buf), "This trace message should not be printed")

	logger.Debug("This is a debug message")
	assert.Contains(t, string(sink.buf), "This is a debug message")

	logger.Info("This is an info message")
	assert.Contains(t, string(sink.buf), "This is an info message")

	logger.Warn("This is a warn message")
	assert.Contains(t, string(sink.buf), "This is a warn message")

	logger.Error("This is an error message")
	assert.Contains(t, string(sink.buf), "This is an error message")
}

func TestConsoleLoggerSinkInfoLevel(t *testing.T) {
	sink := &testSink{}
	logger := NewConsoleLogger().(*consoleLogger).WithSink(sink).(*consoleLogger)
	logger.SetLogLevel(LevelInfo)

	logger.Trace("This trace message should not be printed")
	assert.NotContains(t, string(sink.buf), "This trace message should not be printed")

	logger.Debug("This debug message should not be printed")
	assert.NotContains(t, string(sink.buf), "This debug message should not be printed")

	logger.Info("This is an info message")
	assert.Contains(t, string(sink.buf), "This is an info message")

	logger.Warn("This is a warn message")
	assert.Contains(t, string(sink.buf), "This is a warn message")

	logger.Error("This is an error message")
	assert.Contains(t, string(sink.buf), "This is an error message")
}

func TestConsoleLoggerSinkWarnLevel(t *testing.T) {
	sink := &testSink{}
	logger := NewConsoleLogger().(*consoleLogger).WithSink(sink).(*consoleLogger)
	logger.SetLogLevel(LevelWarn)

	logger.Trace("This trace message should not be printed")
	assert.NotContains(t, string(sink.buf), "This trace message should not be printed")

	logger.Debug("This debug message should not be printed")
	assert.NotContains(t, string(sink.buf), "This debug message should not be printed")

	logger.Info("This info message should not be printed")
	assert.NotContains(t, string(sink.buf), "This info message should not be printed")

	logger.Warn("This is a warn message")
	assert.Contains(t, string(sink.buf), "This is a warn message")

	logger.Error("This is an error message")
	assert.Contains(t, string(sink.buf), "This is an error message")
}

func TestConsoleLoggerSinkErrorLevel(t *testing.T) {
	sink := &testSink{}
	logger := NewConsoleLogger().(*consoleLogger).WithSink(sink).(*consoleLogger)
	logger.SetLogLevel(LevelError)

	logger.Trace("This trace message should not be printed")
	assert.NotContains(t, string(sink.buf), "This trace message should not be printed")

	logger.Debug("This debug message should not be printed")
	assert.NotContains(t, string(sink.buf), "This debug message should not be printed")

	logger.Info("This info message should not be printed")
	assert.NotContains(t, string(sink.buf), "This info message should not be printed")

	logger.Warn("This warn message should not be printed")
	assert.NotContains(t, string(sink.buf), "This warn message should not be printed")

	logger.Error("This is an error message")
	assert.Contains(t, string(sink.buf), "This is an error message")
}
