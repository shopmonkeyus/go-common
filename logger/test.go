package logger

import (
	"context"
	"os"
	"sync"
)

type TestLogEntry struct {
	Severity  string
	Message   string
	Arguments []interface{}
}

type TestLogger struct {
	mu       *sync.Mutex
	metadata map[string]interface{}
	Logs     []TestLogEntry
	root     *TestLogger
}

var _ Logger = (*TestLogger)(nil)

func NewTestLogger() *TestLogger {
	return &TestLogger{
		mu: &sync.Mutex{},
	}
}

func (c *TestLogger) WithSink(sink Sink, level LogLevel) Logger {
	return c
}

// WithPrefix will return a new logger with a prefix prepended to the message
func (c *TestLogger) WithPrefix(prefix string) Logger {
	return c
}

func (c *TestLogger) WithFields(args ...interface{}) Logger {
	return c.With(KV(args...))
}

func (c *TestLogger) With(metadata map[string]interface{}) Logger {
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
	root := c.root
	if root == nil {
		root = c
	}
	return &TestLogger{
		mu:       c.mu,
		metadata: kv,
		root:     root,
	}
}

func (c *TestLogger) Log(level string, msg string, args ...interface{}) {
	if c.mu == nil {
		c.mu = &sync.Mutex{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.root != nil {
		c.root.Logs = append(c.root.Logs, TestLogEntry{level, msg, args})
	} else {
		c.Logs = append(c.Logs, TestLogEntry{level, msg, args})
	}
}

func (c *TestLogger) Trace(msg string, args ...interface{}) {
	c.Log("TRACE", msg, args...)
}

func (c *TestLogger) Debug(msg string, args ...interface{}) {
	c.Log("DEBUG", msg, args...)
}

func (c *TestLogger) Info(msg string, args ...interface{}) {
	c.Log("INFO", msg, args...)
}

func (c *TestLogger) Warn(msg string, args ...interface{}) {
	c.Log("WARN", msg, args...)
}

func (c *TestLogger) Error(msg string, args ...interface{}) {
	c.Log("ERROR", msg, args...)
}

func (c *TestLogger) Fatal(msg string, args ...interface{}) {
	c.Log("FATAL", msg, args...)
	os.Exit(1)
}

func (c *TestLogger) WithContext(ctx context.Context) Logger {
	return c
}

func (c *TestLogger) Flush() error {
	return nil
}
