package logging

import (
	"bytes"
	"context"
	"testing"
)

// Default is never nil, even before SetDefault is called.
func TestDefault_NeverNil(t *testing.T) {
	if Default() == nil {
		t.Fatal("Default() is nil")
	}
}

// SetDefault installs a logger; FromContext returns it as the fallback.
func TestSetDefault_AndFromContextFallback(t *testing.T) {
	prev := Default()
	t.Cleanup(func() { SetDefault(prev) })

	var buf bytes.Buffer
	SetDefault(New(WithWriter(&buf)))

	// No logger on the context -> the package default is used.
	FromContext(context.Background()).Info("via default")

	rec := decodeJSON(t, &buf)
	if rec["msg"] != "via default" {
		t.Errorf("msg = %v, want 'via default'", rec["msg"])
	}
}

// SetDefault ignores a nil logger, leaving the current default in place.
func TestSetDefault_NilIgnored(t *testing.T) {
	prev := Default()
	t.Cleanup(func() { SetDefault(prev) })

	var buf bytes.Buffer
	want := New(WithWriter(&buf))
	SetDefault(want)
	SetDefault(nil)

	if Default() != want {
		t.Error("SetDefault(nil) replaced the default; want it left unchanged")
	}
}

// NewContext stores a logger; FromContext returns it over the default.
func TestNewContext_AndFromContext(t *testing.T) {
	prev := Default()
	t.Cleanup(func() { SetDefault(prev) })

	var defaultBuf, ctxBuf bytes.Buffer
	SetDefault(New(WithWriter(&defaultBuf)))
	ctxLogger := New(WithWriter(&ctxBuf))

	ctx := NewContext(context.Background(), ctxLogger)
	FromContext(ctx).Info("via ctx")

	if defaultBuf.Len() != 0 {
		t.Errorf("default logger received output: %q", defaultBuf.String())
	}
	rec := decodeJSON(t, &ctxBuf)
	if rec["msg"] != "via ctx" {
		t.Errorf("msg = %v, want 'via ctx'", rec["msg"])
	}
}

// NewContext with a nil logger leaves the context unchanged.
func TestNewContext_NilLogger(t *testing.T) {
	base := context.Background()
	if got := NewContext(base, nil); got != base {
		t.Error("NewContext(ctx, nil) returned a different context; want unchanged")
	}
}

// FromContext tolerates a nil context (returns the default).
func TestFromContext_NilContext(t *testing.T) {
	if FromContext(nil) == nil { //nolint:staticcheck // nil ctx is the case under test
		t.Fatal("FromContext(nil) = nil, want the default logger")
	}
}

// End-to-end: a ctx logger plus ctx-carried fields injects both.
func TestFromContext_InjectsCtxFields(t *testing.T) {
	var buf bytes.Buffer
	ctxLogger := New(WithWriter(&buf))

	ctx := NewContext(context.Background(), ctxLogger)
	ctx = WithCorrelationID(ctx, "req-9")
	ctx = WithTenant(ctx, "globex")

	FromContext(ctx).InfoContext(ctx, "done")

	rec := decodeJSON(t, &buf)
	if rec[CorrelationIDKey] != "req-9" {
		t.Errorf("%s = %v, want req-9", CorrelationIDKey, rec[CorrelationIDKey])
	}
	if rec[TenantKey] != "globex" {
		t.Errorf("%s = %v, want globex", TenantKey, rec[TenantKey])
	}
}
