package console

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/pleme-io/borealis/theme"
	"github.com/pleme-io/logging-go"
)

// The console Sink wired through logging.WithSink renders a themed line that
// carries the message and attrs.
func TestSink_RendersMessageAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{Format: "console"},
		logging.WithWriter(&buf),
		logging.WithSink(Sink(theme.Default())),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("hello world", "k", "v")

	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("console output missing message: %q", out)
	}
	if !strings.Contains(out, "k") || !strings.Contains(out, "v") {
		t.Errorf("console output missing attr k=v: %q", out)
	}
	if !strings.Contains(out, "INFO") {
		t.Errorf("console output missing level label: %q", out)
	}
}

// The console sink honours the level filter from the live level var.
func TestSink_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{Format: "console", Level: "warn"},
		logging.WithWriter(&buf),
		logging.WithSink(Sink(theme.Default())),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("dropped")
	if buf.Len() != 0 {
		t.Errorf("info should be dropped at warn level: %q", buf.String())
	}
	logger.Warn("kept")
	if !strings.Contains(buf.String(), "kept") {
		t.Errorf("warn should be emitted: %q", buf.String())
	}

	// Live retarget: SetLevel should re-enable info on the console.
	buf.Reset()
	if !logging.SetLevel(logger, slog.LevelDebug) {
		t.Fatal("SetLevel failed to recover level var on console logger")
	}
	logger.Info("now visible")
	if !strings.Contains(buf.String(), "now visible") {
		t.Errorf("info should appear after SetLevel(debug): %q", buf.String())
	}
}

// Injected context fields (correlation_id) still appear on console output, since
// the ContextHandler inject stage wraps the console sink.
func TestSink_InjectsContextFields(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{Format: "console"},
		logging.WithWriter(&buf),
		logging.WithSink(Sink(theme.Default())),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	ctx := logging.WithCorrelationID(context.Background(), "req-99")
	logger.InfoContext(ctx, "handled")
	if !strings.Contains(buf.String(), "req-99") {
		t.Errorf("console output missing injected correlation_id: %q", buf.String())
	}
}

// WithGroup flattens grouped attrs onto dotted keys in the single-line console.
func TestSink_GroupedAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{Format: "console"},
		logging.WithWriter(&buf),
		logging.WithSink(Sink(theme.Default())),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.WithGroup("http").Info("served", "status", 200)
	out := buf.String()
	if !strings.Contains(out, "http.status") {
		t.Errorf("expected dotted group key http.status: %q", out)
	}
}
