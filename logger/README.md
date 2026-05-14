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

Eight levels, controlled via the `SM_LOG_LEVEL` environment variable (case-insensitive, defaults to `debug`):

`trace` | `debug` | `info` | `warn` | `error` | `panic` | `fatal` | `none`

> **Note:** The zap logger defaults to `info` when `SM_LOG_LEVEL` is not set (see [Zap Logger](#zap-logger-recommended) below). Other implementations default to `debug`.

```go
level := logger.ParseLogLevel("info")
level := logger.GetLevelFromEnv() // reads SM_LOG_LEVEL
```

## Implementations

### Zap Logger (recommended)

High-performance structured logger built on [uber-go/zap](https://github.com/uber-go/zap). Produces JSON output suitable for production and Cloud Logging.

```go
// Default — Info level, GCP trace correlation enabled
log := logger.NewZapLogger()

// With explicit level
log := logger.NewZapLogger(logger.WithLevel(logger.LevelDebug))

// With initial fields
log := logger.NewZapLogger(logger.WithFields(map[string]interface{}{
    "service": "api",
    "version": "1.2.0",
}))

// Disable GCP trace correlation
log := logger.NewZapLogger(logger.WithGCPTraceCorrelation(false))
```

#### Default Log Level

The zap logger defaults to **Info** when no level is specified and `SM_LOG_LEVEL` is not set. This differs from other implementations which default to Debug. The priority is:

1. `WithLevel(...)` option — highest priority
2. `SM_LOG_LEVEL` environment variable
3. **Info** — fallback default

#### GCP Trace Correlation

GCP trace correlation is **enabled by default**. When enabled, `WithContext(ctx)` and `FromContext(ctx)` automatically enrich log entries with OpenTelemetry trace/span IDs as GCP-compatible fields:

- `logging.googleapis.com/trace`
- `logging.googleapis.com/spanId`
- `logging.googleapis.com/trace_sampled`

This allows Cloud Logging to correlate log entries with distributed traces. To disable:

```go
log := logger.NewZapLogger(logger.WithGCPTraceCorrelation(false))
```

`NewZapGCloudLogger()` is an alias for `NewZapLogger()` — both behave identically.

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

Store and retrieve loggers from `context.Context`. `FromContext` automatically enriches logs with OpenTelemetry trace/span IDs (since GCP trace correlation is enabled by default).

```go
// Store logger in context
ctx = logger.ToContext(ctx, log)

// Retrieve (returns default zap logger if none stored)
// Automatically adds trace/span IDs from context
log := logger.FromContext(ctx)

// Manual trace enrichment (same as what FromContext does)
log = log.WithContext(ctx)
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
