package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	CyanBold    = "\033[36;1m"
	Purple      = "\u001b[38;5;200m"
)

type consoleLogger struct {
	prefix            string
	metadata          map[string]interface{}
	traceLevelColor   string
	traceMessageColor string
	debugLevelColor   string
	debugMessageColor string
	infoLevelColor    string
	infoMessageColor  string
	warnLevelColor    string
	warnMessageColor  string
	errorLevelColor   string
	errorMessageColor string
}

var _ Logger = (*consoleLogger)(nil)

func (c *consoleLogger) Default(val string, def string) string {
	if val == "" {
		return def
	}
	return val
}

func (c *consoleLogger) Clone(kv map[string]interface{}) *consoleLogger {
	return &consoleLogger{
		metadata:          kv,
		prefix:            c.prefix,
		traceLevelColor:   c.Default(c.traceLevelColor, CyanBold),
		traceMessageColor: c.Default(c.traceMessageColor, Gray),
		debugLevelColor:   c.Default(c.debugLevelColor, BlueBold),
		debugMessageColor: c.Default(c.debugMessageColor, Green),
		infoLevelColor:    c.Default(c.infoLevelColor, YellowBold),
		infoMessageColor:  c.Default(c.infoMessageColor, WhiteBold),
		warnLevelColor:    c.Default(c.infoMessageColor, MagentaBold),
		warnMessageColor:  c.Default(c.warnMessageColor, Magenta),
		errorLevelColor:   c.Default(c.errorMessageColor, RedBold),
		errorMessageColor: c.Default(c.errorMessageColor, Red),
	}
}

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
	if prefix, found := kv["prefix"]; found {
		delete(kv, "prefix")
		l := c.Clone(kv)
		l.prefix = prefix.(string)
		return l
	}
	return c.Clone(kv)
}

func (c *consoleLogger) Log(levelColor string, messageColor string, levelString string, msg string, args ...interface{}) {
	_msg := fmt.Sprintf(msg, args...)
	var prefix string
	var suffix string
	if c.prefix != "" {
		prefix = Purple + c.prefix + Reset + " "
	}
	if c.metadata != nil {
		buf, _ := json.Marshal(c.metadata)
		_buf := string(buf)
		if _buf != "{}" {
			suffix = " " + Gray + _buf + Reset
		}
	}
	var levelSuffix string
	if len(levelString) < 5 {
		levelSuffix = strings.Repeat(" ", 5-len(levelString))
	}
	level := levelColor + fmt.Sprintf("[%s]%s", levelString, levelSuffix) + Reset
	message := messageColor + _msg + Reset
	log.Printf("%s %s%s%s\n", level, prefix, message, suffix)
}

func (c *consoleLogger) Trace(msg string, args ...interface{}) {
	c.Log(c.traceLevelColor, c.traceMessageColor, "TRACE", msg, args...)
}

func (c *consoleLogger) Debug(msg string, args ...interface{}) {
	c.Log(c.debugLevelColor, c.debugMessageColor, "DEBUG", msg, args...)
}

func (c *consoleLogger) Info(msg string, args ...interface{}) {
	c.Log(c.infoLevelColor, c.infoMessageColor, "INFO", msg, args...)
}

func (c *consoleLogger) Warn(msg string, args ...interface{}) {
	c.Log(c.warnLevelColor, c.warnMessageColor, "WARN", msg, args...)
}

func (c *consoleLogger) Error(msg string, args ...interface{}) {
	c.Log(c.errorLevelColor, c.errorMessageColor, "ERROR", msg, args...)
}

// NewConsoleLogger returns a new Logger instance which will log to the console
func NewConsoleLogger() Logger {
	return (&consoleLogger{}).Clone(nil)
}
