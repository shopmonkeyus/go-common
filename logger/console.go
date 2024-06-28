package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	gstrings "github.com/shopmonkeyus/go-common/string"
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

// LogLevel defines the level of logging
type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

type consoleLogger struct {
	prefixes          []string
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
	sink              Sink
	logLevel          LogLevel
}

var _ Logger = (*consoleLogger)(nil)

func (c *consoleLogger) Default(val string, def string) string {
	if val == "" {
		return def
	}
	return val
}

// WithPrefix will return a new logger with a prefix prepended to the message
func (c *consoleLogger) WithPrefix(prefix string) Logger {
	prefixes := make([]string, 0)
	prefixes = append(prefixes, c.prefixes...)
	if !gstrings.Contains(prefixes, prefix, false) {
		prefixes = append(prefixes, prefix)
	}
	l := c.Clone(c.metadata, c.sink)
	l.prefixes = prefixes
	return l
}

var isCI = os.Getenv("CI") != ""

func (c *consoleLogger) Clone(kv map[string]interface{}, sink Sink) *consoleLogger {
	prefixes := make([]string, 0)
	prefixes = append(prefixes, c.prefixes...)
	var tracecolor = Gray
	if isCI {
		tracecolor = Purple
	}
	return &consoleLogger{
		metadata:          kv,
		prefixes:          prefixes,
		traceLevelColor:   c.Default(c.traceLevelColor, CyanBold),
		traceMessageColor: c.Default(c.traceMessageColor, tracecolor),
		debugLevelColor:   c.Default(c.debugLevelColor, BlueBold),
		debugMessageColor: c.Default(c.debugMessageColor, Green),
		infoLevelColor:    c.Default(c.infoLevelColor, YellowBold),
		infoMessageColor:  c.Default(c.infoMessageColor, WhiteBold),
		warnLevelColor:    c.Default(c.infoMessageColor, MagentaBold),
		warnMessageColor:  c.Default(c.warnMessageColor, Magenta),
		errorLevelColor:   c.Default(c.errorMessageColor, RedBold),
		errorMessageColor: c.Default(c.errorMessageColor, Red),
		sink:              sink,
	}
}

func (c *consoleLogger) WithSink(sink Sink) Logger {
	c.sink = sink
	return c
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
	if len(kv) == 0 {
		kv = nil
	}
	return c.Clone(kv, c.sink)
}

func (c *consoleLogger) Log(level LogLevel, levelColor string, messageColor string, levelString string, msg string, args ...interface{}) {
	if level < c.logLevel {
		return
	}
	_msg := fmt.Sprintf(msg, args...)
	var prefix string
	var suffix string
	if len(c.prefixes) > 0 {
		prefix = Purple + strings.Join(c.prefixes, " ") + Reset + " "
	}
	if c.metadata != nil {
		buf, _ := json.Marshal(c.metadata)
		_buf := string(buf)
		if _buf != "{}" {
			if isCI {
				suffix = " " + MagentaBold + _buf + Reset
			} else {
				suffix = " " + Gray + _buf + Reset
			}
		}
	}
	var levelSuffix string
	if len(levelString) < 5 {
		levelSuffix = strings.Repeat(" ", 5-len(levelString))
	}
	levelText := levelColor + fmt.Sprintf("[%s]%s", levelString, levelSuffix) + Reset
	message := messageColor + _msg + Reset
	out := fmt.Sprintf("%s %s%s%s", levelText, prefix, message, suffix)
	log.Printf("%s\n", out)
	if c.sink != nil {
		c.sink.Write([]byte(ansiColorStripper.ReplaceAllString(out, "")))
	}
}

func (c *consoleLogger) Trace(msg string, args ...interface{}) {
	c.Log(LevelTrace, c.traceLevelColor, c.traceMessageColor, "TRACE", msg, args...)
}

func (c *consoleLogger) Debug(msg string, args ...interface{}) {
	c.Log(LevelDebug, c.debugLevelColor, c.debugMessageColor, "DEBUG", msg, args...)
}

func (c *consoleLogger) Info(msg string, args ...interface{}) {
	c.Log(LevelInfo, c.infoLevelColor, c.infoMessageColor, "INFO", msg, args...)
}

func (c *consoleLogger) Warn(msg string, args ...interface{}) {
	c.Log(LevelWarn, c.warnLevelColor, c.warnMessageColor, "WARN", msg, args...)
}

func (c *consoleLogger) Error(msg string, args ...interface{}) {
	c.Log(LevelError, c.errorLevelColor, c.errorMessageColor, "ERROR", msg, args...)
}

func (c *consoleLogger) SetLogLevel(level LogLevel) {
	c.logLevel = level
}

// NewConsoleLogger returns a new Logger instance which will log to the console
func NewConsoleLogger(levels ...LogLevel) Logger {
	if len(levels) > 0 {
		return (&consoleLogger{logLevel: levels[0]}).Clone(nil, nil)
	}
	return (&consoleLogger{logLevel: LevelDebug}).Clone(nil, nil)
}
