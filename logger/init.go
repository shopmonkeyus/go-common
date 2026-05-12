package logger

import (
	"context"
	"io"
	"os"
	"regexp"
	"strings"
)

// LogLevel defines the level of logging
type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelNone
)

// ParseLogLevel converts a string to a LogLevel. Case-insensitive.
// Returns LevelDebug for unrecognized values.
func ParseLogLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "none":
		return LevelNone
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelDebug
	}
}

// GetLevelFromEnv reads the SM_LOG_LEVEL environment variable and converts it to a LogLevel.
func GetLevelFromEnv() LogLevel {
	return ParseLogLevel(os.Getenv("SM_LOG_LEVEL"))
}

type Sink io.Writer

// Logger is an interface for logging
type Logger interface {
	// With will return a new logger using metadata as the base context
	With(metadata map[string]interface{}) Logger
	// WithFields will return a new logger with the given key-value pairs as context
	WithFields(args ...interface{}) Logger
	// WithPrefix will return a new logger with a prefix prepended to the message
	WithPrefix(prefix string) Logger
	// WithContext returns a new logger enriched with context information (e.g., trace IDs)
	WithContext(ctx context.Context) Logger
	// Trace level logging
	Trace(msg string, args ...interface{})
	// Debug level logging
	Debug(msg string, args ...interface{})
	// Info level logging
	Info(msg string, args ...interface{})
	// Warning level logging
	Warn(msg string, args ...interface{})
	// Error level logging
	Error(msg string, args ...interface{})
	// Fatal level logging and exit with code 1
	Fatal(msg string, args ...interface{})
	// Flush flushes any buffered log entries
	Flush() error
}

type SinkLogger interface {
	Logger
	// SetSink will set the sink, and level to sink
	SetSink(sink Sink, level LogLevel)
}

var ansiColorStripper = regexp.MustCompile("\x1b\\[[0-9;]*[mK]")
