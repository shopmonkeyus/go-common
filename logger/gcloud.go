package logger

import (
	"encoding/json"
	"fmt"
	"log"
)

// Entry defines a log entry
// https://github.com/GoogleCloudPlatform/golang-samples/blob/08bc985b4973901c09344eabbe9d7d5add7dc656/run/logging-manual/main.go
type Entry struct {
	Message  string                 `json:"message"`
	Severity string                 `json:"severity,omitempty"`
	Trace    string                 `json:"logging.googleapis.com/trace,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
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
	metadata  map[string]interface{}
	traceID   string
	component string
}

var _ Logger = (*gcloudLogger)(nil)

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
	return &gcloudLogger{kv, traceID, component}
}

func (c *gcloudLogger) Log(severity string, msg string, args ...interface{}) {
	_msg := msg
	if len(args) > 0 {
		_msg = fmt.Sprintf(msg, args...)
	}
	log.Println(Entry{
		Severity:  severity,
		Message:   _msg,
		Trace:     c.traceID,
		Metadata:  c.metadata,
		Component: c.component,
	})
}

func (c *gcloudLogger) Trace(msg string, args ...interface{}) {
	c.Log("TRACE", msg, args...)
}

func (c *gcloudLogger) Debug(msg string, args ...interface{}) {
	c.Log("DEBUG", msg, args...)
}

func (c *gcloudLogger) Info(msg string, args ...interface{}) {
	c.Log("INFO", msg, args...)
}

func (c *gcloudLogger) Warn(msg string, args ...interface{}) {
	c.Log("WARNING", msg, args...)
}

func (c *gcloudLogger) Error(msg string, args ...interface{}) {
	c.Log("ERROR", msg, args...)
}

// NewGCloudLogger returns a new Logger instance which can be used for structured google cloud logging
func NewGCloudLogger() Logger {
	return &gcloudLogger{}
}
