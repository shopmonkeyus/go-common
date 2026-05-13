# logger

A structured logging package with multiple backend implementations, context propagation, and GCP Cloud Logging support.

## Logger Interface

All implementations satisfy the `Logger` interface:

```go
type Logger interface {
    With(metadata map[string]interface{}) Logger
    WithFields(args ...interface{}) Logger
    WithPrefix(prefix string) Logger
    WithContext(ctx context.Context) Logger
    Trace(msg string, args ...interface{})
    Debug(msg string, args ...interface{})
    Info(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
    Error(msg string, args ...interface{})
    Fatal(msg string, args ...interface{})
    Flush() error
}
```

## Log Levels

Six levels, controlled via the `SM_LOG_LEVEL` environment variable (case-insensitive, defaults to `debug`):

`trace` | `debug` | `info` | `warn` | `error` | `none`

```go
level := logger.ParseLogLevel("info")
level := logger.GetLevelFromEnv() // reads SM_LOG_LEVEL
```

## Implementations

### Zap Logger (recommended)

High-performance structured logger built on [uber-go/zap](https://github.com/uber-go/zap). Produces JSON output suitable for production and Cloud Logging.

```go
// Default — reads SM_LOG_LEVEL from env
log := logger.NewZapLogger()

// With explicit level
log := logger.NewZapLogger(logger.WithLevel(logger.LevelInfo))

// With initial fields
log := logger.NewZapLogger(logger.WithFields(map[string]interface{}{
    "service": "api",
    "version": "1.2.0",
}))

// GCP Cloud Logging with trace correlation
log := logger.NewZapGCloudLogger()
// or manually:
log := logger.NewZapLogger(logger.WithGCPTraceCorrelation())
```

#### Sampling

Zap enables sampling by default to protect against log flooding. Sampling is per-second, per-message (same level + message text):

- **Initial: 100** — the first 100 entries are always logged
- **Thereafter: 100** — after the initial 100, every 100th entry is logged; the rest are dropped

This means a single log line must fire >100 times per second before any entries are dropped. To disable sampling:

```go
log := logger.NewZapLogger(logger.WithSampling(nil))
```

### Console Logger

Colorized terminal output for local development. Supports sinks for writing logs to an additional `io.Writer`.

```go
log := logger.NewConsoleLogger()                    // level from SM_LOG_LEVEL
log := logger.NewConsoleLogger(logger.LevelTrace)   // explicit level

// Attach a sink (e.g. file)
sinkLog := logger.NewConsoleLogger()
sinkLog.SetSink(file, logger.LevelDebug)
```

### JSON Logger

Structured JSON output compatible with GCP Cloud Logging format.

```go
log := logger.NewJSONLogger()
log := logger.NewGCloudLogger()                     // alias for NewJSONLogger
log := logger.NewJSONLoggerWithSink(sink, logger.LevelInfo) // sink-only, no console
```

### Multi Logger

Fan-out to multiple loggers simultaneously.

```go
log := logger.NewMultiLogger(
    logger.NewConsoleLogger(),
    logger.NewZapLogger(),
)
```

### Test Logger

Captures log entries in memory for assertions in tests.

```go
log := logger.NewTestLogger()
log.Info("hello %s", "world")

entry := log.Logs[0]
// entry.Severity == "INFO"
// entry.Message  == "hello %s"
```

## Enriching Logs

```go
// Structured metadata (map)
log = log.With(map[string]interface{}{"requestID": "abc-123"})

// Key-value pairs (variadic)
log = log.WithFields("user", "alice", "action", "login")

// Named prefix (shows as component/named logger)
log = log.WithPrefix("auth")
```

## Context Integration

Store and retrieve loggers from `context.Context`. `FromContext` automatically enriches logs with OpenTelemetry trace/span IDs when GCP trace correlation is enabled.

```go
// Store logger in context
ctx = logger.ToContext(ctx, log)

// Retrieve (returns default zap logger if none stored)
log := logger.FromContext(ctx)

// Manual trace enrichment
log = log.WithContext(ctx) // adds trace_id, span_id, trace_sampled for GCP
```

## Flushing

Call `Flush()` before application exit to ensure buffered entries are written (important for the zap logger).

```go
defer log.Flush()
```

## Utilities

`KV` converts variadic key-value pairs into a `map[string]interface{}`:

```go
m := logger.KV("key1", "val1", "key2", 42)
// map[string]interface{}{"key1": "val1", "key2": 42}
```
