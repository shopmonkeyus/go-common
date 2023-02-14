package logger

import (
	"encoding/json"
	"fmt"
	"log"
)

const (
	Reset       = "\033[0m"
	Gray        = "\033[1;30m"
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	BlueBold    = "\033[34;1m"
	MagentaBold = "\033[35;1m"
	RedBold     = "\033[31;1m"
	YellowBold  = "\033[33;1m"
	WhiteBold   = "\033[37;1m"
)

type consoleLogger struct {
	metadata map[string]interface{}
}

var _ Logger = (*consoleLogger)(nil)

func (c *consoleLogger) With(metadata map[string]interface{}) Logger {
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
	return &consoleLogger{kv}
}

func (c *consoleLogger) Log(severityLabel string, msg string, args ...interface{}) {
	_msg := fmt.Sprintf(msg, args...)
	if c.metadata != nil {
		buf, _ := json.Marshal(c.metadata)
		log.Printf("[%s] %s %s\n", severityLabel, Green+_msg, Gray+string(buf)+Reset)
	} else {
		log.Printf("[%s] %s\n", severityLabel, Green+_msg+Reset)
	}
}

func (c *consoleLogger) Trace(msg string, args ...interface{}) {
	c.Log(Blue+"TRACE"+Reset, msg, args...)
}

func (c *consoleLogger) Debug(msg string, args ...interface{}) {
	c.Log(Blue+"DEBUG"+Reset, msg, args...)
}

func (c *consoleLogger) Info(msg string, args ...interface{}) {
	c.Log(Yellow+"INFO"+Reset, msg, args...)
}

func (c *consoleLogger) Warn(msg string, args ...interface{}) {
	c.Log(Magenta+"WARNING"+Reset, msg, args...)
}

func (c *consoleLogger) Error(msg string, args ...interface{}) {
	c.Log(Red+"ERROR"+Reset, msg, args...)
}

// NewConsoleLogger returns a new Logger instance which will log to the console
func NewConsoleLogger() Logger {
	return &consoleLogger{}
}
