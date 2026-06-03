package logging

import (
	"fmt"
	"log/slog"
)

// Redacted is the placeholder substituted for any value the logger must not
// render in plaintext. It is the single, stable redaction marker shared by the
// [SecretString] LogValuer and the declarative redaction middleware in the
// logging-go/redact sub-package, so log aggregators see one consistent token.
const Redacted = "[REDACTED]"

// SecretString wraps a sensitive string — an Akeyless token, an API key, a
// password — so that logging it can never leak the plaintext. It implements
// [slog.LogValuer] (and [fmt.Stringer] / [fmt.GoStringer]) to return [Redacted]
// in every rendering path, making plaintext token logging structurally hard
// rather than relying on every call site to remember to redact.
//
// This is the zero-dep, opt-in companion to the declarative key-based redaction
// in the logging-go/redact sub-package: SecretString protects a value wherever
// it travels (you log the typed value), while redact protects by attribute key
// (you redact at the sink regardless of how the value was typed). Use both —
// type the value AND redact the key.
//
//	tok := logging.SecretString(rawToken)
//	logging.FromContext(ctx).InfoContext(ctx, "authenticated", "token", tok)
//	// → ...,"token":"[REDACTED]"   (never the plaintext)
//
// The underlying plaintext is still reachable for legitimate use via [Reveal];
// only the logging/stringer paths are redacted.
type SecretString string

// LogValue implements [slog.LogValuer], yielding the [Redacted] marker so the
// secret is never serialized by any slog handler.
func (s SecretString) LogValue() slog.Value { return slog.StringValue(Redacted) }

// String implements [fmt.Stringer], so a SecretString printed with %s / %v —
// the common accidental-leak path — also shows [Redacted].
func (s SecretString) String() string { return Redacted }

// GoString implements [fmt.GoStringer], so even %#v formatting redacts.
func (s SecretString) GoString() string { return fmt.Sprintf("logging.SecretString(%q)", Redacted) }

// Reveal returns the underlying plaintext for legitimate non-logging use (e.g.
// sending the token to the API it authenticates). It is the deliberate, named
// escape hatch — grep-auditable — so revealing a secret is always an explicit
// act, never an accident of formatting.
func (s SecretString) Reveal() string { return string(s) }
