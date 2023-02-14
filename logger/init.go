package logger

// Logger is an interface for logging
type Logger interface {
	// With will return a new logger using metadata as the base context
	With(metadata map[string]interface{}) Logger
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
