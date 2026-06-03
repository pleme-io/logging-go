package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// decodeJSON parses the single JSON log line in buf. It fails the test if the
// buffer is empty or holds more than one line.
func decodeJSON(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Fatal("no log output")
	}
	if strings.Count(out, "\n") != 0 {
		t.Fatalf("expected exactly one log line, got:\n%s", out)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return rec
}

// JSON is the default format; output is parseable and carries the message.
func TestNew_JSONDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf))

	logger.Info("hello", "k", "v")

	rec := decodeJSON(t, &buf)
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", rec["msg"])
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
	if rec["k"] != "v" {
		t.Errorf("k = %v, want v", rec["k"])
	}
}

// Injected correlation_id and tenant from ctx appear in JSON output.
func TestNew_InjectsContextFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf))

	ctx := WithCorrelationID(context.Background(), "req-123")
	ctx = WithTenant(ctx, "acme")

	logger.InfoContext(ctx, "handled", "status", 200)

	rec := decodeJSON(t, &buf)
	if rec[CorrelationIDKey] != "req-123" {
		t.Errorf("%s = %v, want req-123", CorrelationIDKey, rec[CorrelationIDKey])
	}
	if rec[TenantKey] != "acme" {
		t.Errorf("%s = %v, want acme", TenantKey, rec[TenantKey])
	}
	// JSON numbers decode as float64.
	if rec["status"] != float64(200) {
		t.Errorf("status = %v, want 200", rec["status"])
	}
}

// Without ctx fields (or via the non-Context method), no injected keys appear.
func TestNew_NoContextFields(t *testing.T) {
	tests := []struct {
		name string
		emit func(l *slog.Logger)
	}{
		{
			name: "plain Info passes background ctx",
			emit: func(l *slog.Logger) { l.Info("plain") },
		},
		{
			name: "InfoContext with empty ctx",
			emit: func(l *slog.Logger) { l.InfoContext(context.Background(), "empty") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(WithWriter(&buf))
			tt.emit(logger)
			rec := decodeJSON(t, &buf)
			if _, ok := rec[CorrelationIDKey]; ok {
				t.Errorf("unexpected %s in %v", CorrelationIDKey, rec)
			}
			if _, ok := rec[TenantKey]; ok {
				t.Errorf("unexpected %s in %v", TenantKey, rec)
			}
		})
	}
}

// Only one of the two ctx fields set -> only that field is injected.
func TestNew_PartialContextFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf))

	ctx := WithCorrelationID(context.Background(), "only-id")
	logger.InfoContext(ctx, "partial")

	rec := decodeJSON(t, &buf)
	if rec[CorrelationIDKey] != "only-id" {
		t.Errorf("%s = %v, want only-id", CorrelationIDKey, rec[CorrelationIDKey])
	}
	if _, ok := rec[TenantKey]; ok {
		t.Errorf("unexpected %s in %v", TenantKey, rec)
	}
}

// Level filtering: records below the configured level are dropped.
func TestNew_LevelFiltering(t *testing.T) {
	tests := []struct {
		name    string
		level   slog.Level
		emit    func(l *slog.Logger)
		wantOut bool
	}{
		{"warn drops info", slog.LevelWarn, func(l *slog.Logger) { l.Info("x") }, false},
		{"warn drops debug", slog.LevelWarn, func(l *slog.Logger) { l.Debug("x") }, false},
		{"warn keeps warn", slog.LevelWarn, func(l *slog.Logger) { l.Warn("x") }, true},
		{"warn keeps error", slog.LevelWarn, func(l *slog.Logger) { l.Error("x") }, true},
		{"debug keeps info", slog.LevelDebug, func(l *slog.Logger) { l.Info("x") }, true},
		{"debug keeps debug", slog.LevelDebug, func(l *slog.Logger) { l.Debug("x") }, true},
		{"info drops debug", slog.LevelInfo, func(l *slog.Logger) { l.Debug("x") }, false},
		{"error drops warn", slog.LevelError, func(l *slog.Logger) { l.Warn("x") }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(WithWriter(&buf), WithLevel(tt.level))
			tt.emit(logger)
			gotOut := strings.TrimSpace(buf.String()) != ""
			if gotOut != tt.wantOut {
				t.Errorf("output present = %v, want %v (buf=%q)", gotOut, tt.wantOut, buf.String())
			}
		})
	}
}

// Env-var level parsing: WithLevelFromEnv reads and applies the level.
func TestNew_LevelFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envName   string
		envVal    string
		setEnv    bool
		debugOut  bool // is a Debug() record emitted?
		infoOut   bool // is an Info() record emitted?
	}{
		{name: "debug enables debug", envName: "LOG_LEVEL", envVal: "debug", setEnv: true, debugOut: true, infoOut: true},
		{name: "warn drops info", envName: "LOG_LEVEL", envVal: "warn", setEnv: true, debugOut: false, infoOut: false},
		{name: "error drops info", envName: "LOG_LEVEL", envVal: "error", setEnv: true, debugOut: false, infoOut: false},
		{name: "custom env name", envName: "MY_LEVEL", envVal: "debug", setEnv: true, debugOut: true, infoOut: true},
		{name: "unset keeps info default", envName: "LOG_LEVEL", setEnv: false, debugOut: false, infoOut: true},
		{name: "garbage keeps info default", envName: "LOG_LEVEL", envVal: "loud", setEnv: true, debugOut: false, infoOut: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.envName, tt.envVal)
			} else {
				// An empty value parses to an error, so the level is left at
				// the info default — the same outcome as an unset variable for
				// our purposes. t.Setenv restores the prior value at test end.
				t.Setenv(tt.envName, "")
			}

			var buf bytes.Buffer
			logger := New(WithWriter(&buf), WithLevelFromEnv(tt.envName))

			buf.Reset()
			logger.Debug("d")
			if gotDebug := strings.TrimSpace(buf.String()) != ""; gotDebug != tt.debugOut {
				t.Errorf("debug emitted = %v, want %v", gotDebug, tt.debugOut)
			}

			buf.Reset()
			logger.Info("i")
			if gotInfo := strings.TrimSpace(buf.String()) != ""; gotInfo != tt.infoOut {
				t.Errorf("info emitted = %v, want %v", gotInfo, tt.infoOut)
			}
		})
	}
}

// WithLevelFromEnv with an empty name falls back to LOG_LEVEL.
func TestWithLevelFromEnv_EmptyNameUsesDefault(t *testing.T) {
	t.Setenv(DefaultLevelEnv, "debug")
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithLevelFromEnv(""))
	logger.Debug("d")
	if strings.TrimSpace(buf.String()) == "" {
		t.Error("debug record dropped; WithLevelFromEnv(\"\") did not read LOG_LEVEL")
	}
}

// Explicit WithLevel after WithLevelFromEnv wins (last option applied).
func TestNew_OptionOrderLastWins(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var buf bytes.Buffer
	// env says debug, but the explicit WithLevel(warn) comes last.
	logger := New(WithWriter(&buf), WithLevelFromEnv("LOG_LEVEL"), WithLevel(slog.LevelWarn))
	logger.Info("i")
	if strings.TrimSpace(buf.String()) != "" {
		t.Error("info emitted; explicit WithLevel(warn) should have won over env debug")
	}
}

// Text format: output is key=value, not JSON, and still carries ctx fields.
func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithFormat("text"))

	ctx := WithTenant(context.Background(), "acme")
	logger.InfoContext(ctx, "hi", "k", "v")

	out := buf.String()
	if json.Valid([]byte(strings.TrimSpace(out))) {
		t.Errorf("expected non-JSON text output, got %q", out)
	}
	if !strings.Contains(out, "msg=hi") {
		t.Errorf("text output missing msg=hi: %q", out)
	}
	if !strings.Contains(out, "k=v") {
		t.Errorf("text output missing k=v: %q", out)
	}
	if !strings.Contains(out, TenantKey+"=acme") {
		t.Errorf("text output missing %s=acme: %q", TenantKey, out)
	}
}

// WithFormat is case-insensitive and ignores unknown values (keeps JSON).
func TestWithFormat_Normalisation(t *testing.T) {
	tests := []struct {
		format   string
		wantJSON bool
	}{
		{"JSON", true},
		{"json", true},
		{"TEXT", false},
		{"text", false},
		{"bogus", true}, // unknown -> JSON default preserved
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(WithWriter(&buf), WithFormat(tt.format))
			logger.Info("m")
			gotJSON := json.Valid([]byte(strings.TrimSpace(buf.String())))
			if gotJSON != tt.wantJSON {
				t.Errorf("format %q: gotJSON=%v want %v (buf=%q)", tt.format, gotJSON, tt.wantJSON, buf.String())
			}
		})
	}
}

// WithAddSource adds a source attribute with file/line.
func TestWithAddSource(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf), WithAddSource(true))
	logger.Info("m")

	rec := decodeJSON(t, &buf)
	src, ok := rec[slog.SourceKey].(map[string]any)
	if !ok {
		t.Fatalf("missing %s attr in %v", slog.SourceKey, rec)
	}
	if _, ok := src["file"]; !ok {
		t.Errorf("source missing file: %v", src)
	}
	if _, ok := src["line"]; !ok {
		t.Errorf("source missing line: %v", src)
	}
}

// WithAddSource off (default) emits no source attribute.
func TestWithAddSource_DefaultOff(t *testing.T) {
	var buf bytes.Buffer
	logger := New(WithWriter(&buf))
	logger.Info("m")
	rec := decodeJSON(t, &buf)
	if _, ok := rec[slog.SourceKey]; ok {
		t.Errorf("unexpected %s attr: %v", slog.SourceKey, rec)
	}
}

// nil and nil-writer options are tolerated (defaults preserved).
func TestNew_NilOptionsAndWriter(t *testing.T) {
	logger := New(nil, WithWriter(nil))
	if logger == nil {
		t.Fatal("New returned nil logger")
	}
	// Should not panic; defaults to stdout JSON. Just exercise it.
	logger.Info("safe")
}
