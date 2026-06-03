# logging-go

Go representation of pleme-io's structured-logging convention. The Go
counterpart to the Rust [`tracing`](https://docs.rs/tracing) +
[`tracing-subscriber`](https://docs.rs/tracing-subscriber) stack and
`pleme-actions-shared::log`: the same model, so every Go service and tool
emits logs the same way.

> **Pure standard library core.** Built entirely on [`log/slog`](https://pkg.go.dev/log/slog).
> Zero external dependencies — offline-buildable with a minimal closure.
> Dep-bearing features (borealis console, declarative redaction) live in
> quarantined leaf sub-modules (`console/`, `redact/`) so the core stays clean.

## What & why

Ad-hoc `fmt.Println` logging and bespoke per-service formats make logs
impossible to aggregate, correlate, or filter at fleet scale. This library
gives every Go program one logger shape:

- **Construct once** with [`New`](logging.go) using functional options.
- **JSON to stdout** by default (machine-parseable), text on demand.
- **Level from env** — `LOG_LEVEL` (`debug`/`info`/`warn`/`error`), like the
  Rust stack's single well-known level variable.
- **Context-carried fields** — a correlation ID and tenant set on the context
  are injected into *every* record, the Go analog of tracing's span fields.

## Surface

| Symbol | Purpose |
| --- | --- |
| `New(opts ...Option) *slog.Logger` | Build a logger; JSON/stdout/info by default. |
| `WithLevel(slog.Level)` | Set the minimum level explicitly. |
| `WithLevelFromEnv(name string)` | Read the level from an env var (default `LOG_LEVEL`). |
| `WithWriter(io.Writer)` | Redirect output (default stdout). |
| `WithFormat("json"\|"text"\|"console"\|"discard")` | Choose the sink. |
| `WithAddSource(bool)` | Annotate records with source location. |
| `WithDiscard()` | Route to `slog.DiscardHandler` (no-op sink for tests/disabled). |
| `Config` + `FromConfig(cfg, opts…) (*slog.Logger, error)` | Typed yaml config → logger (§3.5 canonical config consumer; never calls `shikumi.Load`). |
| `Middleware = func(slog.Handler) slog.Handler` + `WithMiddleware(…)` + `Pipe(sink, …)` | Decorator pipeline (Law 2): `redact → inject → sample → sink`. |
| `FieldExtractor` + `WithExtractors(…)` | Pluggable ctx→attrs registry; `CorrelationIDExtractor`/`TenantExtractor` are the built-ins. |
| `WithLevelVar(*slog.LevelVar)` / `SetLevel(logger, lvl)` / `LevelVarOf(logger)` | Live, dynamic verbosity (zap.AtomicLevel pattern). |
| `SecretString` + `Redacted` | `slog.LogValuer` that redacts a secret in every render path. |
| `WithSink(SinkFunc)` | Inject a gated terminal sink (e.g. the borealis console) without the core importing it (Law 8). |
| `WithCorrelationID(ctx, id)` | Carry a correlation ID on the context. |
| `WithTenant(ctx, tenant)` | Carry a tenant on the context. |
| `ContextHandler` | `slog.Handler` wrapper that injects the ctx fields (group-aware). |
| `FromContext(ctx) *slog.Logger` | Logger from ctx, falling back to the default. |
| `Default()` / `SetDefault(*slog.Logger)` | The package-level logger. |
| `ParseLevel(name) (slog.Level, error)` | Parse a level name. |

### Leaf sub-modules (gated deps)

| Module | Dep | Purpose |
| --- | --- | --- |
| `logging-go/console` | `pleme-io/borealis` | Themed dev/CLI console sink — `console.Sink(theme)` → a `logging.SinkFunc`, wired via `logging.WithSink`. Level glyph + `comp.KV` attrs. |
| `logging-go/redact` | `samber/slog-formatter` | Declarative key-based redaction `Middleware` — `redact.Keys(…)`, `redact.PII(…)`, `redact.FormatKey(…)`. |

```go
// Typed config + gated console + declarative redaction, composed:
log, _ := logging.FromConfig(cfg.Logging,
	logging.WithSink(console.Sink(theme.Default())),     // gated borealis console
	logging.WithMiddleware(redact.Keys("token", "password")), // gated redaction
)
logging.SetLevel(log, slog.LevelDebug) // live verbosity change
```

## Usage

```go
package main

import (
	"context"

	"github.com/pleme-io/logging-go"
)

func main() {
	logger := logging.New(
		logging.WithLevelFromEnv("LOG_LEVEL"), // debug/info/warn/error
		logging.WithFormat("json"),            // or "text"
	)
	logging.SetDefault(logger)

	// Carry request-scoped fields on the context.
	ctx := logging.WithCorrelationID(context.Background(), "req-123")
	ctx = logging.WithTenant(ctx, "acme")

	// correlation_id + tenant are injected into the record from ctx.
	// Use the *Context methods so the active context reaches the handler.
	logging.FromContext(ctx).InfoContext(ctx, "handled request", "status", 200)
	// {"time":"...","level":"INFO","msg":"handled request",
	//  "status":200,"correlation_id":"req-123","tenant":"acme"}
}
```

The fields only attach to records emitted through slog's `*Context` methods
(`InfoContext`, `ErrorContext`, …), since those carry the active context to
the handler. Plain `Info`/`Error` calls pass a background context and so carry
no injected fields.

## Build & test

```bash
go build ./...
go test ./...
```
