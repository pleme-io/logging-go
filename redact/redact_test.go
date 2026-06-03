package redact

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/pleme-io/logging-go"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	return rec
}

// Keys redacts the value under each named key with the stable marker, and never
// leaks the plaintext.
func TestKeys_Redacts(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{},
		logging.WithWriter(&buf),
		logging.WithMiddleware(Keys("token", "password")),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("auth", "token", "super-secret", "password", "hunter2", "user", "alice")

	rec := decode(t, &buf)
	if rec["token"] != logging.Redacted {
		t.Errorf("token = %v, want %s", rec["token"], logging.Redacted)
	}
	if rec["password"] != logging.Redacted {
		t.Errorf("password = %v, want %s", rec["password"], logging.Redacted)
	}
	if rec["user"] != "alice" {
		t.Errorf("non-secret field should pass through: user = %v", rec["user"])
	}
	if strings.Contains(buf.String(), "super-secret") || strings.Contains(buf.String(), "hunter2") {
		t.Errorf("plaintext secret leaked: %q", buf.String())
	}
}

// FormatKey applies a custom partial-mask formatter.
func TestFormatKey_PartialMask(t *testing.T) {
	var buf bytes.Buffer
	last4 := func(v slog.Value) slog.Value {
		s := v.String()
		if len(s) > 4 {
			return slog.StringValue("****" + s[len(s)-4:])
		}
		return slog.StringValue("****")
	}
	logger, err := logging.FromConfig(
		logging.Config{},
		logging.WithWriter(&buf),
		logging.WithMiddleware(FormatKey("card", last4)),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("charge", "card", "4111111111111234")
	rec := decode(t, &buf)
	if rec["card"] != "****1234" {
		t.Errorf("card = %v, want ****1234", rec["card"])
	}
}

// PII masks structured values under the key while leaving non-PII intact.
func TestPII_Masks(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{},
		logging.WithWriter(&buf),
		logging.WithMiddleware(PII("email")),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	logger.Info("signup", "email", "alice@example.com")
	if strings.Contains(buf.String(), "alice@example.com") {
		t.Errorf("PII email leaked: %q", buf.String())
	}
}

// Redaction composes with the context inject stage: correlation_id still lands.
func TestKeys_ComposesWithInject(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.FromConfig(
		logging.Config{},
		logging.WithWriter(&buf),
		logging.WithMiddleware(Keys("token")),
	)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	ctx := logging.WithCorrelationID(t.Context(), "req-7")
	logger.InfoContext(ctx, "auth", "token", "leak-me")
	rec := decode(t, &buf)
	if rec["token"] != logging.Redacted {
		t.Errorf("token not redacted: %v", rec)
	}
	if rec[logging.CorrelationIDKey] != "req-7" {
		t.Errorf("correlation_id missing after redaction middleware: %v", rec)
	}
}
