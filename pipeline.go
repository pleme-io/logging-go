package logging

import "log/slog"

// Middleware is a [slog.Handler] decorator: it wraps an inner handler and
// returns a new handler that adds one cross-cutting concern (redaction,
// sampling, field injection, fan-out, …) before delegating. It is the universal
// composition seam Law 2 mandates — we compose over slog's own Handler
// interface rather than inventing a bespoke owner type — and matches
// samber/slog-multi's `Middleware = func(slog.Handler) slog.Handler`, so any
// middleware from that ecosystem (slog-sampling's Threshold/Uniform,
// slog-formatter's FormatByKey/PIIFormatter) drops straight into our pipeline.
//
// The canonical pipeline order, innermost-sink-last, is:
//
//	redact → inject (ContextHandler) → sample → sink
//
// expressed as a slice of Middleware applied over the format sink. Earlier
// entries wrap later ones, so the first Middleware is the outermost decorator
// and runs first on each record.
type Middleware = func(slog.Handler) slog.Handler

// Pipe composes middlewares over a base sink handler, returning a single
// [slog.Handler]. Middlewares are applied so the first listed is the outermost
// (runs first); a nil middleware is skipped. With no middlewares it returns the
// sink unchanged.
//
//	h := logging.Pipe(sink, redactMW, injectMW, sampleMW)
//	// record flows: redactMW → injectMW → sampleMW → sink
//
// This is the explicit, testable form of the algebra [New] applies internally;
// callers wiring a handler by hand (e.g. for a slog-multi Fanout) use it
// directly.
func Pipe(sink slog.Handler, middlewares ...Middleware) slog.Handler {
	h := sink
	// Apply in reverse so the first listed middleware ends up outermost.
	for i := len(middlewares) - 1; i >= 0; i-- {
		if mw := middlewares[i]; mw != nil {
			h = mw(h)
		}
	}
	return h
}
