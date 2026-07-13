# Zap Logger Caller-Skip Design

Date: 2026-07-13
Repo: `go-common`

## Problem

The Zap-backed `Logger` (`logger/zap.go`) wraps `*zap.SugaredLogger`. Each level
method calls the sugared logger from inside the wrapper:

```go
func (z *zapLogger) Info(msg string, args ...interface{}) {
    z.sugar.Infof(msg, args...)
}
```

Zap annotates each entry with the *direct* caller of the sugared method. Because
that direct caller is always `zapLogger.Info/Debug/...` in `logger/zap.go`, the
`caller` field always points at the go-common wrapper instead of the real
application call site. Consumers (e.g. the `changefeed` service) therefore see
`logger/zap.go:NN` on every log line, which is useless for locating the origin.

Additional facts:

- `NewZapLogger` builds via `zapCfg.Build()` (production config), which *does*
  emit a `caller` field — currently pointing at `zap.go`.
- `NewZapTestLogger` builds via a bare `zap.New(core, ...)` with no
  `zap.AddCaller()`, so it emits **no** caller field at all today. The current
  tests do not observe caller output.

## Goal

Report the true application call site by default, and allow consumers that add
their own wrapper layer to skip additional frames.

## Design

### 1. Baseline fix (default behavior)

In `buildZapLogger`, apply caller options to the base logger before `.Sugar()`:

```go
const defaultCallerSkip = 1 // skips the go-common zapLogger.<Level> wrapper frame

base = base.WithOptions(zap.AddCaller(), zap.AddCallerSkip(defaultCallerSkip + cfg.callerSkip))
```

- `defaultCallerSkip = 1` skips the single go-common wrapper frame
  (`zapLogger.Info/Debug/...`), so `caller` points at the real call site.
- All three constructors — `NewZapLogger`, `NewZapGCloudLogger` (delegates to
  `NewZapLogger`), and `NewZapTestLogger` — funnel through `buildZapLogger`, so
  the fix applies uniformly.
- `zap.AddCaller()` is idempotent-safe to add here; applying it in
  `buildZapLogger` also gives `NewZapTestLogger` a caller field for the first
  time (previously absent).
- Consumers get correct caller reporting with **zero code changes**.

### 2. Configurable option (opt-in extra skip)

Add a new `ZapOption`, mirroring `zap.AddCallerSkip` naming:

```go
// AddCallerSkip adds n additional stack frames to skip when reporting the
// caller. Use it when this logger is wrapped by an extra layer of your own so
// the reported caller points at your call site rather than your wrapper.
func AddCallerSkip(n int) ZapOption {
    return func(c *zapConfig) {
        c.callerSkip += n
    }
}
```

- Additive semantics: the total skip is `defaultCallerSkip + cfg.callerSkip`.
  Callers reason about *their own* wrapper layers, not go-common internals.
- New field on `zapConfig`: `callerSkip int` (defaults to 0).

### 3. Test logger emits caller

Because `buildZapLogger` now adds `zap.AddCaller()`, `NewZapTestLogger` emits a
`caller` field. The test encoder already uses
`zap.NewProductionEncoderConfig()`, which sets `CallerKey: "caller"`, so no
encoder change is required — the field appears automatically once
`zap.AddCaller()` is applied.

### 4. Tests

- Extend the test's `zapLogEntry` struct with a `Caller string json:"caller"`
  field.
- New test: default logger reports a `caller` containing `zap_test.go` (the
  actual call site) — NOT `zap.go`.
- New test: `AddCallerSkip(1)` shifts the reported frame one level up (verify
  the caller changes when an extra wrapper frame is introduced, or that an
  out-of-range skip degrades gracefully to zap's `undefined` marker).

### 5. Changefeed

No functional change required — `changefeed` calls the go-common `Logger`
directly (`log.Info(...)`), so the baseline fix corrects its caller output
automatically. Verification only: confirm it builds against the updated
`go-common` and that log lines now reference changefeed source files.

## Non-goals

- Do **not** add `AddCallerSkip` to the shared `Logger` interface. This stays a
  Zap-specific `ZapOption`; the JSON, multi, and nop loggers are untouched.
- No runtime/chainable caller-skip method on the `Logger` interface.

## Files touched

- `logger/zap.go` — `zapConfig.callerSkip`, `AddCallerSkip` option,
  `buildZapLogger` caller options, `defaultCallerSkip` const.
- `logger/zap_test.go` — caller field in `zapLogEntry`, new caller tests.
- (verification only) `changefeed` — build/behavior check, no code change
  expected.
