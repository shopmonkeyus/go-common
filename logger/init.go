package logger

import "regexp"

type Sink interface {
	// Write will receive the log output as a slice of bytes
	Write([]byte) error
}

// Logger is an interface for logging
type Logger interface {
	// With will return a new logger using metadata as the base context
	With(metadata map[string]interface{}) Logger
	// WithPrefix will return a new logger with a prefix prepended to the message
	WithPrefix(prefix string) Logger
	// WithSink returns a Logger which will also delegate logs to the provided sink
	WithSink(sink Sink) Logger
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
}

var ansiColorStripper = regexp.MustCompile("\x1b\\[[0-9;]*[mK]")
