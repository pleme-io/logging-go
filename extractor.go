package logging

import (
	"context"
	"log/slog"
)

// FieldExtractor pulls zero or more [slog.Attr] from a context, to be injected
// at the top level of every record by a [ContextHandler]. It is the pluggable
// generalization of the two originally-hardcoded context fields: correlation_id
// and tenant are now ordinary built-in extractors ([CorrelationIDExtractor],
// [TenantExtractor]), and new sources — trace_id/span_id from an OTel span,
// a request ID, a user ID — opt in by registering another extractor.
//
// An extractor MUST be cheap and allocation-light: it runs on every record that
// passes the level filter. It MUST NOT mutate the context or block. Returning a
// nil/empty slice when nothing applies is the expected fast path.
//
// This mirrors the Rust tracing stack, where span fields are likewise a
// registry of layers enriching every event, not two hardcoded columns.
type FieldExtractor func(ctx context.Context) []slog.Attr

// CorrelationIDExtractor injects the [CorrelationIDKey] attribute from a context
// carrying a correlation ID set via [WithCorrelationID]. It is one of the two
// built-in extractors installed by default on every [New]/[FromConfig] logger.
func CorrelationIDExtractor(ctx context.Context) []slog.Attr {
	if id, ok := CorrelationIDFromContext(ctx); ok {
		return []slog.Attr{slog.String(CorrelationIDKey, id)}
	}
	return nil
}

// TenantExtractor injects the [TenantKey] attribute from a context carrying a
// tenant set via [WithTenant]. It is one of the two built-in extractors
// installed by default on every [New]/[FromConfig] logger.
func TenantExtractor(ctx context.Context) []slog.Attr {
	if tenant, ok := TenantFromContext(ctx); ok {
		return []slog.Attr{slog.String(TenantKey, tenant)}
	}
	return nil
}

// defaultExtractors returns a fresh slice of the built-in extractors, in the
// stable order they are applied: correlation ID, then tenant. A fresh slice is
// returned each call so callers never alias the package's backing storage.
func defaultExtractors() []FieldExtractor {
	return []FieldExtractor{CorrelationIDExtractor, TenantExtractor}
}

// runExtractors collects the attributes produced by the given extractors for
// ctx, preserving extractor order. Nil-returning extractors and a nil context
// are tolerated. The result is nil when nothing applies — the fast path the
// [ContextHandler] uses to skip injection entirely.
func runExtractors(extractors []FieldExtractor, ctx context.Context) []slog.Attr {
	if ctx == nil || len(extractors) == 0 {
		return nil
	}
	var attrs []slog.Attr
	for _, ex := range extractors {
		if ex == nil {
			continue
		}
		if got := ex(ctx); len(got) > 0 {
			attrs = append(attrs, got...)
		}
	}
	return attrs
}
