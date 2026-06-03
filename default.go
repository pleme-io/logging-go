package logging

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// defaultLogger holds the package-level logger returned by [Default] and
// [FromContext]. It is read lock-free via atomic.Pointer; [SetDefault] swaps in
// a new value atomically (the slog/ArcSwap analog).
var defaultLogger atomic.Pointer[slog.Logger]

// loggerCtxKey is the context key under which a per-request logger may be
// stored via [NewContext], so [FromContext] can return it in preference to the
// package default.
type loggerCtxKey struct{}

func init() {
	// Seed the package default so [Default] and [FromContext] never return nil,
	// even before [SetDefault] is called. Uses the standard New defaults: JSON
	// to stdout at info level, with LOG_LEVEL honoured if set.
	defaultLogger.Store(New(WithLevelFromEnv("")))
}

// Default returns the package-level logger. It is never nil. Replace it with
// [SetDefault]; reads are lock-free.
func Default() *slog.Logger {
	return defaultLogger.Load()
}

// SetDefault atomically installs logger as the package-level default returned by
// [Default] and used as the fallback by [FromContext]. A nil logger is ignored,
// leaving the current default in place.
func SetDefault(logger *slog.Logger) {
	if logger != nil {
		defaultLogger.Store(logger)
	}
}

// NewContext returns a child context carrying logger, so a downstream
// [FromContext] returns it instead of the package default. This lets a request
// boundary install a logger pre-bound with request-scoped attributes (via
// logger.With(...)) that flows with the context. A nil logger leaves the
// context unchanged.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerCtxKey{}, logger)
}

// FromContext returns the logger associated with ctx, or the package [Default]
// when none is present (including for a nil context). The returned logger is
// never nil.
//
// Pair it with the *Context log methods so context-carried fields are injected:
//
//	logging.FromContext(ctx).InfoContext(ctx, "message", "key", value)
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(loggerCtxKey{}).(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	return Default()
}
