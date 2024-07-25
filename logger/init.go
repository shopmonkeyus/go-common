package logger

import (
	"io"
	"regexp"
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

type Sink io.Writer

// Logger is an interface for logging
type Logger interface {
	// With will return a new logger using metadata as the base context
	With(metadata map[string]interface{}) Logger
	// WithPrefix will return a new logger with a prefix prepended to the message
	WithPrefix(prefix string) Logger
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
}

type SinkLogger interface {
	Logger
	// SetSink will set the sink, and level to sink
	SetSink(sink Sink, level LogLevel)
}

var ansiColorStripper = regexp.MustCompile("\x1b\\[[0-9;]*[mK]")
