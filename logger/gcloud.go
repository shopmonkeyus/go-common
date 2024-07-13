package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// Entry defines a log entry
// https://github.com/GoogleCloudPlatform/golang-samples/blob/08bc985b4973901c09344eabbe9d7d5add7dc656/run/logging-manual/main.go
type Entry struct {
	Timestamp time.Time              `json:"timestamp,omitempty"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity,omitempty"`
	Trace     string                 `json:"logging.googleapis.com/trace,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	// Logs Explorer allows filtering and display of this as `jsonPayload.component`.
	Component string `json:"component,omitempty"`
}

// String renders an entry structure to the JSON format expected by Cloud Logging.
func (e Entry) String() string {
	if e.Severity == "" {
		e.Severity = "INFO"
	}
	out, err := json.Marshal(e)
	if err != nil {
		log.Printf("json.Marshal: %v", err)
	}
	return string(out)
}

type gcloudLogger struct {
	metadata     map[string]interface{}
	traceID      string
	component    string
	sink         Sink
	sinkLogLevel LogLevel
	noConsole    bool
	ts           *time.Time // for unit testing
}

var _ Logger = (*gcloudLogger)(nil)

func (c *gcloudLogger) WithSink(sink Sink, level LogLevel) Logger {
	c.sink = sink
	c.sinkLogLevel = level
	return c
}

// WithPrefix will return a new logger with a prefix prepended to the message
func (c *gcloudLogger) WithPrefix(prefix string) Logger {
	newlogger := c.With(nil).(*gcloudLogger)
	if c.component == "" {
		newlogger.component = prefix
	} else {
		if !strings.Contains(c.component, prefix) {
			newlogger.component = c.component + " " + prefix
		}
	}
	return newlogger
}

func (c *gcloudLogger) With(metadata map[string]interface{}) Logger {
	traceID := c.traceID
	component := c.component
	if trace, ok := metadata["trace"].(string); ok {
		traceID = trace
		delete(metadata, "trace")
	}
	if comp, ok := metadata["component"].(string); ok {
		component = comp
		delete(metadata, "component")
	}
	kv := metadata
	if c.metadata != nil {
		kv = make(map[string]interface{})
		for k, v := range c.metadata {
			kv[k] = v
		}
		for k, v := range metadata {
			kv[k] = v
		}
	}
	if len(kv) == 0 {
		kv = nil
	}
	return &gcloudLogger{
		metadata:     kv,
		traceID:      traceID,
		component:    component,
		noConsole:    c.noConsole,
		sink:         c.sink,
		sinkLogLevel: c.sinkLogLevel,
	}
}

var re = regexp.MustCompile(`\[(.*?)\]`)

func (c *gcloudLogger) tokenize(val string) string {
	if re.MatchString(val) {
		vals := make([]string, 0)
		for _, token := range re.FindAllString(val, -1) {
			vals = append(vals, re.ReplaceAllString(token, "$1"))
		}
		return strings.Join(vals, ", ")
	}
	return val
}

func (c *gcloudLogger) Log(level LogLevel, severity string, msg string, args ...interface{}) {
	_msg := msg
	if len(args) > 0 {
		_msg = fmt.Sprintf(msg, args...)
	}
	entry := Entry{
		Severity:  severity,
		Message:   _msg,
		Trace:     c.traceID,
		Metadata:  c.metadata,
		Component: c.tokenize(c.component),
		Timestamp: time.Now(),
	}
	if !c.noConsole {
		log.Println(entry)
	}
	if c.sink != nil && level >= c.sinkLogLevel {
		entry.Message = ansiColorStripper.ReplaceAllString(entry.Message, "")
		if c.ts != nil {
			entry.Timestamp = *c.ts // for testing
		}
		buf, _ := json.Marshal(entry)
		c.sink.Write(buf)
	}
}

func (c *gcloudLogger) Trace(msg string, args ...interface{}) {
	c.Log(LevelTrace, "TRACE", msg, args...)
}

func (c *gcloudLogger) Debug(msg string, args ...interface{}) {
	c.Log(LevelDebug, "DEBUG", msg, args...)
}

func (c *gcloudLogger) Info(msg string, args ...interface{}) {
	c.Log(LevelInfo, "INFO", msg, args...)
}

func (c *gcloudLogger) Warn(msg string, args ...interface{}) {
	c.Log(LevelWarn, "WARNING", msg, args...)
}

func (c *gcloudLogger) Error(msg string, args ...interface{}) {
	c.Log(LevelError, "ERROR", msg, args...)
}

func (c *gcloudLogger) Fatal(msg string, args ...interface{}) {
	c.Log(LevelError, "ERROR", msg, args...)
}

// NewGCloudLogger returns a new Logger instance which can be used for structured google cloud logging
func NewGCloudLogger() Logger {
	return &gcloudLogger{}
}

// NewGCloudLoggerWithSink returns a new Logger instance using a sink and suppressing the console logging
func NewGCloudLoggerWithSink(sink Sink, level LogLevel) Logger {
	return &gcloudLogger{noConsole: true, sink: sink, sinkLogLevel: level}
}
