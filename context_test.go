package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

// Round-trip: WithCorrelationID / WithTenant stored values are read back.
func TestContextRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		set     func(context.Context) context.Context
		get     func(context.Context) (string, bool)
		wantVal string
		wantOK  bool
	}{
		{
			name:    "correlation id set",
			set:     func(c context.Context) context.Context { return WithCorrelationID(c, "abc") },
			get:     CorrelationIDFromContext,
			wantVal: "abc",
			wantOK:  true,
		},
		{
			name:    "tenant set",
			set:     func(c context.Context) context.Context { return WithTenant(c, "acme") },
			get:     TenantFromContext,
			wantVal: "acme",
			wantOK:  true,
		},
		{
			name:    "empty correlation id is a no-op",
			set:     func(c context.Context) context.Context { return WithCorrelationID(c, "") },
			get:     CorrelationIDFromContext,
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "empty tenant is a no-op",
			set:     func(c context.Context) context.Context { return WithTenant(c, "") },
			get:     TenantFromContext,
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "absent correlation id",
			set:     func(c context.Context) context.Context { return c },
			get:     CorrelationIDFromContext,
			wantVal: "",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.set(context.Background())
			got, ok := tt.get(ctx)
			if got != tt.wantVal || ok != tt.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

// nil context is tolerated by the readers.
func TestContextReaders_NilContext(t *testing.T) {
	if v, ok := CorrelationIDFromContext(nil); ok || v != "" { //nolint:staticcheck // nil ctx is the case under test
		t.Errorf("CorrelationIDFromContext(nil) = (%q, %v), want (\"\", false)", v, ok)
	}
	if v, ok := TenantFromContext(nil); ok || v != "" { //nolint:staticcheck // nil ctx is the case under test
		t.Errorf("TenantFromContext(nil) = (%q, %v), want (\"\", false)", v, ok)
	}
}

// WithCorrelationID overwrites a prior value (last write wins).
func TestWithCorrelationID_Overwrite(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "first")
	ctx = WithCorrelationID(ctx, "second")
	if v, _ := CorrelationIDFromContext(ctx); v != "second" {
		t.Errorf("correlation id = %q, want second", v)
	}
}

// ContextHandler.Enabled delegates to the inner handler's level.
func TestContextHandler_EnabledDelegates(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := NewContextHandler(inner)

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(info) = true, want false (inner is warn)")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(error) = false, want true")
	}
}

// WithAttrs/WithGroup preserve the context-injection behaviour.
func TestContextHandler_WithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	h := NewContextHandler(inner).
		WithAttrs([]slog.Attr{slog.String("service", "api")}).
		WithGroup("g")

	logger := slog.New(h)
	ctx := WithCorrelationID(context.Background(), "cid")
	logger.InfoContext(ctx, "m", "field", 1)

	rec := decodeJSON(t, &buf)
	// The base attribute (added before the group opened) is top-level.
	if rec["service"] != "api" {
		t.Errorf("service = %v, want api", rec["service"])
	}
	// The injected correlation id is added at Handle time, also top-level.
	if rec[CorrelationIDKey] != "cid" {
		t.Errorf("%s = %v, want cid", CorrelationIDKey, rec[CorrelationIDKey])
	}
	// The call-site attribute lands inside the open group.
	group, ok := rec["g"].(map[string]any)
	if !ok {
		t.Fatalf("missing group g in %v", rec)
	}
	if group["field"] != float64(1) {
		t.Errorf("g.field = %v, want 1", group["field"])
	}
}
