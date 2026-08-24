# go-common

> Audience: engineers working in this repo AND Claude Code running here (in IDE or CI). Both need the same guardrails.

## What this module is

`go-common` is the **shared Go library** imported by every Go service at Shopmonkey: `director`, `changefeed`, `manifold`, `depot`, `hitch`, `wrench`, `eds`, `intake`.

It is a **versioned public module**. Two facts follow from that, and they drive every rule below.

## ⚠️ Two things to know before you touch anything

### 1. This repository is PUBLIC

`github.com/shopmonkeyus/go-common` is open on the internet.

**Never commit:** internal hostnames, cluster names, project IDs, credentials, customer names, internal URLs, or anything that describes our private topology. Code review here is also a disclosure review.

### 2. A breaking change ripples to every Go service

There is **no monorepo safety net**. Each service pins its own version, and they are already out of sync:

| Service | Pinned version |
|---|---|
| `changefeed` | `v0.1.2` |
| `manifold` | `v0.0.81` |
| `director` | `v0.0.74` |

Three services, three versions. A change you make today reaches them only when someone bumps, and it reaches them in an order nobody controls.

**So: additive changes only, unless you are deliberately doing a coordinated migration.**

## Core Invariants

Non-negotiable. If a later rule seems to conflict with one of these, the invariant wins.

1. **Public repo — no internal detail.** See above. This is the first check on every diff.

2. **Never break an exported signature.** Add a new function. Add a new option. Do not change what exists. Callers you cannot see are compiled against it.

3. **The `logger` package is the org-wide standard.** Every service's `CLAUDE.md` carries the invariant "shared logger only — no stdlib `log`, no `slog`, no `zap`". That promise lives here. Changing logger behaviour changes how every service logs, including in production incidents.

4. **`dbchange` defines a wire format, and `changefeed` is its only importer.**

   Verified 2026-08-21: `changefeed` imports `go-common/dbchange` in **14 files**. `manifold`, `eds`, `depot`, `wrench` and `director` import it in **none**. Each parses the JSON with its own independently-maintained struct.

   **That makes this package more dangerous to change, not less.** It carries the JSON tags, and `changefeed` marshals it onto the `dbchange.*` subject. So these tags *are* the contract:

   - Rename or retag a field and the emitted JSON changes. **Nothing fails to compile**, because no consumer imports this package.
   - `go mod why` will not find the affected services. Dependency tooling cannot see the coupling at all.
   - The consumer structs have **already drifted**. `eds/internal/dbchange.go` omits `region`, `sessionId` and `version`, and adds its own `imported` flag.

   **A JSON-tag change here is a silent breaking change to four services.** Coordinate with `changefeed`, and read each consumer's own struct by hand — the compiler will not help you.

5. **`nats` is the shared connection and consumer layer.** 1,659 lines that every event-driven service depends on. Reconnect, ack, and error semantics are load-bearing.

6. **No service-specific logic.** If it only makes sense for one service, it belongs in that service. This module is for what is genuinely shared.

7. **Never push to `main`** — all changes go through PRs.

8. **Tag deliberately.** A new tag is what makes a change reachable. Do not tag a partial change.

## Packages

| Package | Lines | What it is |
|---|---|---|
| `logger` | 2,019 | **The org-wide logger.** Console, GCloud, JSON, Multi, Test, and Zap variants |
| `nats` | 1,659 | Shared NATS connection and consumer layer |
| `string` | 719 | String helpers, including the hash used for partitioning |
| `sys` | 703 | Process and shutdown handling — `CreateShutdownChannel` |
| `request` | 671 | HTTP request helpers |
| `analytics` | 562 | Analytics emission |
| `env` | 469 | Environment variable handling |
| `dns` | 381 | DNS helpers |
| `command` | 309 | CLI helpers |
| `dbchange` | 309 | **Wire types for `dbchange.*` events** |
| `schema` | 246 | Schema helpers |
| `cache` | 238 | Caching |
| `gcp` | 148 | GCP helpers, including Secret Manager |
| `compress` | 134 | Compression |
| `slice` | 103 | Slice helpers |
| `intake` | 83 | Intake client |

### The logger constructors, and when each is used

| Constructor | Used when |
|---|---|
| `NewConsoleLogger` | Local development |
| `NewGCloudLogger` | Running in GCP — structured for Cloud Logging |
| `NewJSONLogger` | Structured output to a sink |
| `NewMultiLogger` | Fan out to several sinks |
| `NewTestLogger` | Tests |
| `NewZap*` variants | Zap-backed alternatives |

Services choose at startup based on whether they are in cloud. `director/cmd/root.go` is a clean example.

## Stack

- **Go 1.25.0** (`go.mod`)
- `cockroachdb/errors` — error wrapping
- `cloud.google.com/go/secretmanager` — secret access
- `cespare/xxhash/v2` — hashing

## Quality Gate

Before opening a PR:

1. `go build ./...` passes
2. `go test ./...` passes
3. **No internal hostname, project ID, cluster name, or credential appears in the diff**
4. No exported signature changed. New surface is additive
5. If a `dbchange` **JSON tag** changed, the PR names the coordinating `changefeed` change **and** lists which consumer structs were checked by hand. No consumer imports this package, so nothing will fail to compile
6. If `logger` behaviour changed, the PR names which services are affected
7. New exported functions have a doc comment. This is a public module

## Stop and Confirm

Ask a human before you:

- Change any exported signature
- Change anything in `dbchange`
- Change logger output format, level handling, or field naming
- Change `nats` reconnect, ack, or error semantics
- Change the hash in `string` — `changefeed` partitions on it
- Change shutdown behaviour in `sys`
- Cut a new tag

A wrong change here is not one incident. It is one incident per service, arriving whenever each team bumps.

## Technology Guardrails

- Do not add a heavy dependency. Everything here is inherited by every Go service.
- Do not add a second logging library. The point of this package is that there is one.
- Do not add a package that only one service uses.
- Do not remove anything. Deprecate, document, and leave it.

## Known drift

Recorded 2026-08-21. Fix as you touch.

1. **`README.md` says "Golang 1.19 or later".** `go.mod` says **1.25.0**.
2. **The README's import example is wrong.** It shows `import "github.com/shopmonkeyus/go-common"`, but the module root has no package. Real imports are subpackages, for example `github.com/shopmonkeyus/go-common/logger`.
3. **The README does not mention that this repo is public**, or that consumers pin different versions. Both are the most important facts about it.
4. **This repo is absent from the ownership sheet.** Every Go service depends on it and nobody owns it. Tracked in ELSQ-131.

## Runbooks

Not applicable. This is a library, not a running service.

If a bad release breaks a service, the runbook belongs to that service. The fix here is a new tag plus a version bump downstream.
