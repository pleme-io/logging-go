// Package logging is the Go representation of pleme-io's structured-logging
// convention. It mirrors the Rust `tracing` + `tracing-subscriber` stack and
// `pleme-actions-shared::log`, so Go services and tools emit logs the same way
// everywhere — built entirely on the standard library's [log/slog].
//
// The mandate, like the Rust side: no ad-hoc fmt.Println logging, no bespoke
// per-service log formats. Construct one logger with [New], thread context
// through the call graph, and let context-carried fields (correlation ID,
// tenant) attach to every record automatically.
//
//	logger := logging.New(
//		logging.WithLevelFromEnv("LOG_LEVEL"),
//		logging.WithFormat("json"),
//	)
//	logging.SetDefault(logger)
//
//	ctx := logging.WithCorrelationID(context.Background(), "req-123")
//	ctx = logging.WithTenant(ctx, "acme")
//
//	// correlation_id + tenant are injected into the record from ctx:
//	logging.FromContext(ctx).InfoContext(ctx, "handled request", "status", 200)
//
// The default handler emits JSON to stdout, with the level taken from an option
// or the LOG_LEVEL environment variable (debug/info/warn/error), defaulting to
// info. Use [WithFormat] to switch to the text handler, [WithWriter] to
// redirect output, and [WithAddSource] to annotate records with source
// locations — the same knobs tracing-subscriber exposes.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Format selects the slog handler used to render records.
type Format string

const (
	// FormatJSON renders records with [slog.JSONHandler] — the default,
	// machine-parseable format used in production.
	FormatJSON Format = "json"
	// FormatText renders records with [slog.TextHandler] — a human-readable
	// key=value format handy for local development.
	FormatText Format = "text"
)

// DefaultLevelEnv is the environment variable consulted for the log level when
// [WithLevelFromEnv] is not given an explicit name. It mirrors the Rust stack's
// convention of a single well-known level variable.
const DefaultLevelEnv = "LOG_LEVEL"

// config is the resolved set of options used to build a logger. It is internal;
// callers configure it exclusively through [Option] values passed to [New].
type config struct {
	level     slog.Level
	writer    io.Writer
	format    Format
	addSource bool
}

// Option configures [New] using the functional-options pattern, matching the
// house style across the pleme-io Go libraries.
type Option func(*config)

// WithLevel sets the minimum level a record must meet to be emitted. Records
// below this level are dropped. This takes precedence over any environment
// variable resolved by [WithLevelFromEnv] when both are supplied, with the
// last option applied winning.
func WithLevel(level slog.Level) Option {
	return func(c *config) { c.level = level }
}

// WithLevelFromEnv sets the minimum level from the named environment variable,
// parsed via [ParseLevel] (debug/info/warn/error, case-insensitive). If name is
// empty, [DefaultLevelEnv] ("LOG_LEVEL") is used. If the variable is unset or
// unparseable, the current level is left unchanged (info by default).
func WithLevelFromEnv(name string) Option {
	return func(c *config) {
		if name == "" {
			name = DefaultLevelEnv
		}
		if v, ok := os.LookupEnv(name); ok {
			if lvl, err := ParseLevel(v); err == nil {
				c.level = lvl
			}
		}
	}
}

// WithWriter directs log output to w instead of stdout. A nil writer is
// ignored, leaving the default in place.
func WithWriter(w io.Writer) Option {
	return func(c *config) {
		if w != nil {
			c.writer = w
		}
	}
}

// WithFormat selects the output format: "json" (default) or "text". Any other
// value is ignored, leaving the current format in place. The string form is
// accepted (rather than the [Format] type) so callers can wire it straight from
// a config field or flag.
func WithFormat(format string) Option {
	return func(c *config) {
		switch Format(strings.ToLower(format)) {
		case FormatJSON:
			c.format = FormatJSON
		case FormatText:
			c.format = FormatText
		}
	}
}

// WithAddSource toggles source-location annotation. When enabled, each record
// carries the file and line of the log call (slog's AddSource), mirroring
// tracing's source spans. Off by default.
func WithAddSource(add bool) Option {
	return func(c *config) { c.addSource = add }
}

// New builds a [*slog.Logger] from the given options.
//
// By default it writes JSON to stdout at info level. The level can be set
// explicitly with [WithLevel] or read from the environment with
// [WithLevelFromEnv]; the output writer with [WithWriter]; the format with
// [WithFormat]; and source annotation with [WithAddSource].
//
// The returned logger's handler is wrapped in a [ContextHandler], so any
// context-carried fields attached via [WithCorrelationID] / [WithTenant] are
// injected into every record emitted through the *Context methods
// (InfoContext, ErrorContext, …).
func New(opts ...Option) *slog.Logger {
	cfg := config{
		level:  slog.LevelInfo,
		writer: os.Stdout,
		format: FormatJSON,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     cfg.level,
		AddSource: cfg.addSource,
	}

	var base slog.Handler
	switch cfg.format {
	case FormatText:
		base = slog.NewTextHandler(cfg.writer, handlerOpts)
	default:
		base = slog.NewJSONHandler(cfg.writer, handlerOpts)
	}

	return slog.New(NewContextHandler(base))
}
