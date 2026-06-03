package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// FromConfig with a zero Config yields the New default (JSON/stdout/info).
func TestFromConfig_ZeroIsDefault(t *testing.T) {
	var buf bytes.Buffer
	logger, err := FromConfig(Config{}, WithWriter(&buf))
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("hi", "k", "v")
	rec := decodeJSON(t, &buf)
	if rec["msg"] != "hi" || rec["level"] != "INFO" || rec["k"] != "v" {
		t.Errorf("unexpected record: %v", rec)
	}
}

// FromConfig honours level + format from the typed struct.
func TestFromConfig_LevelAndFormat(t *testing.T) {
	var buf bytes.Buffer
	logger, err := FromConfig(Config{Level: "warn", Format: "text"}, WithWriter(&buf))
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("dropped")
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("info should be dropped at warn level: %q", buf.String())
	}
	logger.Warn("kept", "k", "v")
	out := buf.String()
	if json.Valid([]byte(strings.TrimSpace(out))) {
		t.Errorf("expected text (non-JSON) output: %q", out)
	}
	if !strings.Contains(out, "msg=kept") || !strings.Contains(out, "k=v") {
		t.Errorf("text output missing fields: %q", out)
	}
}

// FromConfig returns an error for an invalid level string.
func TestFromConfig_InvalidLevel(t *testing.T) {
	if _, err := FromConfig(Config{Level: "loud"}); err == nil {
		t.Fatal("expected error for invalid level, got nil")
	}
}

// FromConfig discard format drops every record.
func TestFromConfig_Discard(t *testing.T) {
	var buf bytes.Buffer
	logger, err := FromConfig(Config{Format: "discard"}, WithWriter(&buf))
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Error("nothing should appear")
	if buf.Len() != 0 {
		t.Errorf("discard sink emitted output: %q", buf.String())
	}
}

// WithDiscard routes to the no-op sink.
func TestWithDiscard(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithDiscard())
	logger.Error("x")
	if buf.Len() != 0 {
		t.Errorf("WithDiscard emitted output: %q", buf.String())
	}
}

// A custom FieldExtractor injects a top-level attr from context.
func TestWithExtractors_Custom(t *testing.T) {
	type reqIDKeyT struct{}
	reqIDKey := reqIDKeyT{}
	reqExtractor := func(ctx context.Context) []slog.Attr {
		if v, ok := ctx.Value(reqIDKey).(string); ok && v != "" {
			return []slog.Attr{slog.String("request_id", v)}
		}
		return nil
	}

	var buf bytes.Buffer
	// Keep the built-ins AND add the custom one.
	logger := New(WithWriter(&buf),
		WithExtractors(CorrelationIDExtractor, TenantExtractor, reqExtractor))

	ctx := WithCorrelationID(context.Background(), "req-1")
	ctx = context.WithValue(ctx, reqIDKey, "abc-123")
	logger.InfoContext(ctx, "handled")

	rec := decodeJSON(t, &buf)
	if rec["request_id"] != "abc-123" {
		t.Errorf("request_id = %v, want abc-123", rec["request_id"])
	}
	if rec[CorrelationIDKey] != "req-1" {
		t.Errorf("%s = %v, want req-1", CorrelationIDKey, rec[CorrelationIDKey])
	}
}

// WithExtractors REPLACES the built-in set: omitting tenant drops it.
func TestWithExtractors_Replaces(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithExtractors(CorrelationIDExtractor))

	ctx := WithCorrelationID(context.Background(), "req-1")
	ctx = WithTenant(ctx, "acme")
	logger.InfoContext(ctx, "handled")

	rec := decodeJSON(t, &buf)
	if rec[CorrelationIDKey] != "req-1" {
		t.Errorf("%s = %v, want req-1", CorrelationIDKey, rec[CorrelationIDKey])
	}
	if _, ok := rec[TenantKey]; ok {
		t.Errorf("tenant should be dropped when not in custom extractor set: %v", rec)
	}
}

// The custom extractor's injected field stays top-level even with a group open.
func TestWithExtractors_TopLevelWithGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithExtractors(CorrelationIDExtractor))
	logger = logger.WithGroup("req")

	ctx := WithCorrelationID(context.Background(), "req-1")
	logger.InfoContext(ctx, "handled", "status", 200)

	rec := decodeJSON(t, &buf)
	// correlation_id must be top-level, not nested under "req".
	if rec[CorrelationIDKey] != "req-1" {
		t.Errorf("%s should be top-level = req-1, got %v", CorrelationIDKey, rec[CorrelationIDKey])
	}
	grp, ok := rec["req"].(map[string]any)
	if !ok || grp["status"] != float64(200) {
		t.Errorf("status should be inside req group: %v", rec)
	}
}

// SetLevel retargets a New logger's level live.
func TestSetLevel_Live(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithLevel(slog.LevelInfo))

	logger.Debug("before") // dropped at info
	if strings.TrimSpace(buf.String()) != "" {
		t.Fatalf("debug should be dropped before SetLevel: %q", buf.String())
	}

	if !SetLevel(logger, slog.LevelDebug) {
		t.Fatal("SetLevel returned false; level var not recoverable")
	}
	buf.Reset()
	logger.Debug("after") // now emitted
	if strings.TrimSpace(buf.String()) == "" {
		t.Error("debug should be emitted after SetLevel(debug)")
	}
}

// WithLevelVar shares an externally-owned var that drives the level.
func TestWithLevelVar_Shared(t *testing.T) {
	var buf bytes.Buffer
	lv := new(slog.LevelVar)
	lv.Set(slog.LevelError)
	logger := New(WithWriter(&buf), WithLevelVar(lv))

	logger.Warn("dropped") // below error
	if strings.TrimSpace(buf.String()) != "" {
		t.Fatalf("warn dropped at error level: %q", buf.String())
	}
	lv.Set(slog.LevelWarn) // retarget via the shared var
	buf.Reset()
	logger.Warn("kept")
	if strings.TrimSpace(buf.String()) == "" {
		t.Error("warn should be emitted after lv.Set(warn)")
	}

	if got := LevelVarOf(logger); got != lv {
		t.Errorf("LevelVarOf should return the shared var")
	}
}

// SetLevel still works through a user middleware layer that implements Unwrap.
func TestSetLevel_ThroughMiddleware(t *testing.T) {
	var buf bytes.Buffer
	mw := func(inner slog.Handler) slog.Handler { return passthrough{inner} }
	logger := New(WithWriter(&buf), WithMiddleware(mw))
	if !SetLevel(logger, slog.LevelDebug) {
		t.Fatal("SetLevel could not recover var through transparent middleware")
	}
	logger.Debug("x")
	if strings.TrimSpace(buf.String()) == "" {
		t.Error("debug should emit after SetLevel through middleware")
	}
}

// passthrough is a transparent middleware exposing Unwrap for LevelVarOf.
type passthrough struct{ inner slog.Handler }

func (p passthrough) Enabled(ctx context.Context, l slog.Level) bool { return p.inner.Enabled(ctx, l) }
func (p passthrough) Handle(ctx context.Context, r slog.Record) error { return p.inner.Handle(ctx, r) }
func (p passthrough) WithAttrs(a []slog.Attr) slog.Handler { return passthrough{p.inner.WithAttrs(a)} }
func (p passthrough) WithGroup(n string) slog.Handler      { return passthrough{p.inner.WithGroup(n)} }
func (p passthrough) Unwrap() slog.Handler                 { return p.inner }

// WithMiddleware runs before the inject stage: a rewriting middleware sees the
// record and its rewrite reaches the sink.
func TestWithMiddleware_Order(t *testing.T) {
	var buf bytes.Buffer
	// Middleware that appends a marker attr to every record.
	mark := func(inner slog.Handler) slog.Handler { return marker{inner} }
	logger := New(WithWriter(&buf), WithMiddleware(mark))
	logger.Info("m")
	rec := decodeJSON(t, &buf)
	if rec["marked"] != true {
		t.Errorf("middleware attr missing: %v", rec)
	}
}

type marker struct{ inner slog.Handler }

func (m marker) Enabled(ctx context.Context, l slog.Level) bool { return m.inner.Enabled(ctx, l) }
func (m marker) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.Bool("marked", true))
	return m.inner.Handle(ctx, r)
}
func (m marker) WithAttrs(a []slog.Attr) slog.Handler { return marker{m.inner.WithAttrs(a)} }
func (m marker) WithGroup(n string) slog.Handler      { return marker{m.inner.WithGroup(n)} }

// Pipe composes first-listed-outermost.
func TestPipe_Order(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return func(inner slog.Handler) slog.Handler {
			return orderRec{name: name, inner: inner, log: &order}
		}
	}
	var buf bytes.Buffer
	sink := slog.NewJSONHandler(&buf, nil)
	h := Pipe(sink, mk("a"), mk("b"))
	_ = h.Handle(context.Background(), slog.Record{Level: slog.LevelInfo})
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("expected [a b] (first outermost), got %v", order)
	}
}

type orderRec struct {
	name  string
	inner slog.Handler
	log   *[]string
}

func (o orderRec) Enabled(ctx context.Context, l slog.Level) bool { return true }
func (o orderRec) Handle(ctx context.Context, r slog.Record) error {
	*o.log = append(*o.log, o.name)
	return o.inner.Handle(ctx, r)
}
func (o orderRec) WithAttrs(a []slog.Attr) slog.Handler { return o }
func (o orderRec) WithGroup(n string) slog.Handler      { return o }

// SecretString redacts in slog output and in fmt formatting.
func TestSecretString_Redacts(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf))
	tok := SecretString("super-secret-token")
	logger.Info("auth", "token", tok)

	rec := decodeJSON(t, &buf)
	if rec["token"] != Redacted {
		t.Errorf("token = %v, want %s", rec["token"], Redacted)
	}
	if strings.Contains(buf.String(), "super-secret-token") {
		t.Errorf("plaintext leaked into log: %q", buf.String())
	}

	if got := fmt.Sprintf("%s", tok); got != Redacted {
		t.Errorf("%%s = %q, want %s", got, Redacted)
	}
	if got := fmt.Sprintf("%v", tok); got != Redacted {
		t.Errorf("%%v = %q, want %s", got, Redacted)
	}
	if tok.Reveal() != "super-secret-token" {
		t.Errorf("Reveal did not return plaintext")
	}
	if strings.Contains(fmt.Sprintf("%#v", tok), "super-secret-token") {
		t.Errorf("%%#v leaked plaintext")
	}
}

// Output: stderr writer is selected by Config.Output.
func TestConfig_OutputStderr(t *testing.T) {
	// Swap the package stderr to a buffer for the duration of the test.
	var buf bytes.Buffer
	prev := stderr
	stderr = &buf
	t.Cleanup(func() { stderr = prev })

	logger, err := FromConfig(Config{Output: "stderr"})
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("to-stderr")
	if !strings.Contains(buf.String(), "to-stderr") {
		t.Errorf("expected output on stderr buffer, got %q", buf.String())
	}
}
