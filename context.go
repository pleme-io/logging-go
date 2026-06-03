package logging

import (
	"context"
	"log/slog"
)

// Record attribute keys for the fields injected from context. They are exported
// so consumers can query, redact, or correlate on the exact same key names the
// handler writes — the Go analog of tracing's well-known span fields.
const (
	// CorrelationIDKey is the record attribute key for the correlation ID
	// carried in context via [WithCorrelationID].
	CorrelationIDKey = "correlation_id"
	// TenantKey is the record attribute key for the tenant carried in context
	// via [WithTenant].
	TenantKey = "tenant"
)

// ctxKey is an unexported context-key type, so values stored by this package
// never collide with keys from other packages sharing the same context.
type ctxKey int

const (
	correlationIDCtxKey ctxKey = iota
	tenantCtxKey
)

// WithCorrelationID returns a child context carrying the correlation ID. The ID
// is injected as the [CorrelationIDKey] attribute on every record logged with
// the returned context through a [ContextHandler] (i.e. via the *Context
// methods such as InfoContext). An empty id leaves the context unchanged.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationIDCtxKey, id)
}

// WithTenant returns a child context carrying the tenant identifier. The tenant
// is injected as the [TenantKey] attribute on every record logged with the
// returned context through a [ContextHandler]. An empty tenant leaves the
// context unchanged.
func WithTenant(ctx context.Context, tenant string) context.Context {
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantCtxKey, tenant)
}

// CorrelationIDFromContext returns the correlation ID carried in ctx and whether
// one was set.
func CorrelationIDFromContext(ctx context.Context) (string, bool) {
	return stringFromContext(ctx, correlationIDCtxKey)
}

// TenantFromContext returns the tenant carried in ctx and whether one was set.
func TenantFromContext(ctx context.Context) (string, bool) {
	return stringFromContext(ctx, tenantCtxKey)
}

// stringFromContext reads a string value stored under key, guarding against a
// nil context and a missing or mistyped value.
func stringFromContext(ctx context.Context, key ctxKey) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(key).(string)
	return v, ok && v != ""
}

// ContextHandler is a [slog.Handler] decorator that injects context-carried
// fields — the correlation ID and tenant set via [WithCorrelationID] and
// [WithTenant] — into every record it handles. It is the Go analog of a
// tracing-subscriber layer that enriches events with span fields.
//
// Records emitted through slog's *Context methods (InfoContext, ErrorContext,
// …) carry the active context, so the fields attach automatically. Records
// emitted through the plain methods (Info, Error, …) pass a background context
// and therefore carry no injected fields.
//
// Injected fields are added at the top level of every record, never nested
// inside an open [slog.Logger.WithGroup] group — the location is stable so log
// aggregators can correlate on it unconditionally.
type ContextHandler struct {
	// inner is the active handler, with any WithAttrs/WithGroup already
	// applied. It is used on the fast path, when no context fields need
	// injecting or when no group has been opened.
	inner slog.Handler
	// root is the handler as it stood before the first WithGroup was opened
	// (carrying any pre-group WithAttrs). Context fields are injected through
	// root so they land at the top level, outside any group.
	root slog.Handler
	// ops records the WithAttrs/WithGroup calls applied after the first group
	// opened. Handle replays them on top of root (after injecting the context
	// fields) so call-site attributes still land inside their group while the
	// injected fields stay at the top level. nil until the first group opens.
	ops []handlerOp
}

// handlerOp is a single recorded WithAttrs or WithGroup application, replayed in
// [ContextHandler.Handle] to reconstruct the grouped pipeline on top of root.
type handlerOp struct {
	attrs []slog.Attr // non-nil for a WithAttrs op
	group string      // non-empty for a WithGroup op
}

// NewContextHandler wraps inner so that context-carried fields are injected into
// every record. The returned handler delegates level filtering, attribute
// grouping, and formatting to inner.
func NewContextHandler(inner slog.Handler) *ContextHandler {
	return &ContextHandler{inner: inner, root: inner}
}

// Enabled reports whether the wrapped handler emits records at the given level.
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle injects the context-carried correlation ID and tenant (when present)
// as top-level record attributes, then forwards the record to the wrapped
// handler.
//
// With no group open, the fields are added directly to the record. With a group
// open, the injected fields are bound to the pre-group (root) handler and the
// recorded group/attr operations are replayed on top, so the injected fields
// stay at the top level while call-site attributes remain inside their group.
func (h *ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs := contextAttrs(ctx)
	if len(attrs) == 0 {
		return h.inner.Handle(ctx, record)
	}
	if len(h.ops) == 0 {
		// No group opened: the active handler has no group context, so adding
		// the attributes straight onto the record keeps them top-level.
		record.AddAttrs(attrs...)
		return h.inner.Handle(ctx, record)
	}
	// A group is open: inject at root, then replay the post-root pipeline.
	handler := h.root.WithAttrs(attrs)
	for _, op := range h.ops {
		if op.group != "" {
			handler = handler.WithGroup(op.group)
		} else {
			handler = handler.WithAttrs(op.attrs)
		}
	}
	return handler.Handle(ctx, record)
}

// contextAttrs collects the context-carried fields present in ctx as a slice of
// [slog.Attr], in a stable order (correlation ID then tenant).
func contextAttrs(ctx context.Context) []slog.Attr {
	var attrs []slog.Attr
	if id, ok := CorrelationIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String(CorrelationIDKey, id))
	}
	if tenant, ok := TenantFromContext(ctx); ok {
		attrs = append(attrs, slog.String(TenantKey, tenant))
	}
	return attrs
}

// WithAttrs returns a new [ContextHandler] whose wrapped handler carries the
// given attributes, preserving the context-injection behaviour. Attributes
// added before any group stay top-level; attributes added after a group is open
// land inside that group — matching plain slog handler semantics.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	next := &ContextHandler{inner: h.inner.WithAttrs(attrs), root: h.root}
	if len(h.ops) == 0 {
		// Still ungrouped: advance root in lockstep so these attributes remain
		// top-level for injected fields too.
		next.root = next.inner
	} else {
		next.ops = appendOp(h.ops, handlerOp{attrs: attrs})
	}
	return next
}

// WithGroup returns a new [ContextHandler] whose wrapped handler opens the named
// group, preserving the context-injection behaviour. Injected context fields
// continue to land at the top level, outside the opened group. An empty name is
// a no-op, per the [slog.Handler] contract.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &ContextHandler{
		inner: h.inner.WithGroup(name),
		root:  h.root,
		ops:   appendOp(h.ops, handlerOp{group: name}),
	}
}

// appendOp returns a fresh slice with op appended, never aliasing the input —
// so sibling handlers derived from the same parent never share backing storage.
func appendOp(ops []handlerOp, op handlerOp) []handlerOp {
	out := make([]handlerOp, len(ops)+1)
	copy(out, ops)
	out[len(ops)] = op
	return out
}
