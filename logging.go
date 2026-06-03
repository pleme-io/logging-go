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
	// FormatConsole renders records through the borealis themed console handler
	// (level → comp.Glyph, attrs → comp.KV) for dev/CLI output. The themed sink
	// lives in the gated logging-go/console leaf sub-package (Law 8); selecting
	// it here without installing a console sink via [WithHandlerFunc] falls back
	// to the text handler, so the zero-dep core stays buildable offline.
	FormatConsole Format = "console"
	// FormatDiscard sends records to [slog.DiscardHandler] (Go 1.24), the
	// canonical no-op sink for tests and the disabled/unconfigured path. It
	// drops every record with negligible overhead — the GSDS
	// default-when-unconfigured handler (OBS-12).
	FormatDiscard Format = "discard"
)

// DefaultLevelEnv is the environment variable consulted for the log level when
// [WithLevelFromEnv] is not given an explicit name. It mirrors the Rust stack's
// convention of a single well-known level variable.
const DefaultLevelEnv = "LOG_LEVEL"

// SinkFunc builds the terminal (innermost) [slog.Handler] from the resolved
// writer and handler options. It is the seam through which a gated sink — e.g.
// the borealis console handler in logging-go/console — is injected into the
// core's pipeline without the core importing it (Law 8). When it returns nil
// the core falls back to its built-in sink for the configured format.
type SinkFunc func(w io.Writer, opts *slog.HandlerOptions) slog.Handler

// config is the resolved set of options used to build a logger. It is internal;
// callers configure it exclusively through [Option] values passed to [New].
type config struct {
	level      slog.Level
	levelVar   *slog.LevelVar
	writer     io.Writer
	format     Format
	addSource  bool
	middleware []Middleware
	extractors []FieldExtractor
	sink       SinkFunc
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
		case FormatConsole:
			c.format = FormatConsole
		case FormatDiscard:
			c.format = FormatDiscard
		}
	}
}

// WithAddSource toggles source-location annotation. When enabled, each record
// carries the file and line of the log call (slog's AddSource), mirroring
// tracing's source spans. Off by default.
func WithAddSource(add bool) Option {
	return func(c *config) { c.addSource = add }
}

// WithLevelVar installs an externally-owned [*slog.LevelVar] as the logger's
// level source, enabling live verbosity changes after construction — the zap
// AtomicLevel pattern, and the mechanism by which a shikumi config reload can
// retarget verbosity without rebuilding the logger. The caller retains the
// LevelVar and calls its Set method (or [SetLevel] on the logger returned by
// [New]) to change the level at runtime. A nil var is ignored.
//
// When no LevelVar is supplied, [New] allocates one internally seeded from the
// resolved static level, so [SetLevel]/[LevelVarOf] always work; pass your own
// only when you need to share it across several loggers.
func WithLevelVar(v *slog.LevelVar) Option {
	return func(c *config) {
		if v != nil {
			c.levelVar = v
		}
	}
}

// WithMiddleware appends [Middleware] decorators to the handler pipeline. Each
// is a `func(slog.Handler) slog.Handler`; they are applied in the canonical
// order redact → inject → sample → sink, with the supplied middlewares wrapping
// the built-in [ContextHandler] inject stage (so they see records before
// context fields are injected — the right place for redaction). Middlewares
// from samber/slog-multi, slog-sampling, and slog-formatter compose here
// unchanged. nil middlewares are skipped. Repeated calls accumulate.
func WithMiddleware(mw ...Middleware) Option {
	return func(c *config) {
		for _, m := range mw {
			if m != nil {
				c.middleware = append(c.middleware, m)
			}
		}
	}
}

// WithExtractors sets the [FieldExtractor] registry the [ContextHandler] runs to
// inject top-level fields from context. Passing extractors here REPLACES the
// built-in correlation_id + tenant set, so include [CorrelationIDExtractor] and
// [TenantExtractor] explicitly to keep them. With no call, the built-in set is
// used. Repeated calls accumulate onto the explicit set.
func WithExtractors(extractors ...FieldExtractor) Option {
	return func(c *config) {
		for _, ex := range extractors {
			if ex != nil {
				c.extractors = append(c.extractors, ex)
			}
		}
	}
}

// WithSink injects a terminal [SinkFunc] used to build the innermost handler,
// overriding the format-derived built-in sink. It is how the gated borealis
// console sink (logging-go/console) is wired in without the core importing it
// (Law 8): logging-go/console exposes a [SinkFunc] that callers pass here. A nil
// sink, or a sink that returns nil, falls back to the format-derived built-in.
func WithSink(sink SinkFunc) Option {
	return func(c *config) {
		if sink != nil {
			c.sink = sink
		}
	}
}

// WithDiscard routes output to [slog.DiscardHandler], the no-op sink for tests
// and the disabled path. Equivalent to selecting [FormatDiscard]. It overrides
// any format and writer.
func WithDiscard() Option {
	return func(c *config) { c.format = FormatDiscard }
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

	// The level source is always a *slog.LevelVar so the level can be retargeted
	// live (the zap.AtomicLevel pattern). Use a caller-supplied var when given,
	// else seed an internal one from the resolved static level.
	lv := cfg.levelVar
	if lv == nil {
		lv = new(slog.LevelVar)
		lv.Set(cfg.level)
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     lv,
		AddSource: cfg.addSource,
	}

	// Build the terminal sink: a gated SinkFunc (e.g. the borealis console)
	// wins; otherwise the format selects a built-in stdlib handler.
	var sink slog.Handler
	if cfg.sink != nil {
		sink = cfg.sink(cfg.writer, handlerOpts)
	}
	if sink == nil {
		sink = builtinSink(cfg.format, cfg.writer, handlerOpts)
	}

	// inject stage: context-field injection via the (possibly customized)
	// extractor registry. This is the always-present middleware of the chain.
	// It also carries the live level var so [LevelVarOf]/[SetLevel] can recover
	// it from the returned logger.
	inject := func(h slog.Handler) slog.Handler {
		ch := NewContextHandler(h)
		if len(cfg.extractors) != 0 {
			ch = NewContextHandlerWithExtractors(h, cfg.extractors...)
		}
		ch.levelVar = lv
		return ch
	}

	// Canonical order: caller middleware (e.g. redact) wraps inject wraps sink.
	// Pipe applies first-listed-outermost, so [middleware…, inject] yields
	// middleware → inject → sink.
	chain := append(append([]Middleware{}, cfg.middleware...), inject)
	return slog.New(Pipe(sink, chain...))
}

// builtinSink returns the stdlib handler for a format. FormatConsole without an
// installed console SinkFunc degrades to text so the zero-dep core stays
// buildable offline; FormatDiscard uses the no-op sink.
func builtinSink(format Format, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	switch format {
	case FormatText, FormatConsole:
		return slog.NewTextHandler(w, opts)
	case FormatDiscard:
		return slog.DiscardHandler
	default:
		return slog.NewJSONHandler(w, opts)
	}
}
